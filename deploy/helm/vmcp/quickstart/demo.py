"""Meridian Bank playground helper: users, teams, servers and credentials.

Reads the static demo catalog (works offline) and, for credentials, the
`<release>-quickstart` Secret created by quickstart.sh (one `kubectl get secret`
per invocation). Standard library only. Secrets are printed only by the commands
that explicitly ask for them.
"""
import argparse
import base64
import json
from pathlib import Path
import shutil
import subprocess
import sys

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path: sys.path.insert(0, str(HERE))
import demo_catalog as catalog  # noqa: E402
import demo_upstreams  # noqa: E402

REALM = 'vmcp-demo'
AGENT_CLIENT = 'vmcp-demo-agent'
SECRET_HINT = 'Run quickstart.sh first (with --keycloak for SSO users, --mock for runtime keys), or pass --context/--namespace/--release.'


def parser():
    p = argparse.ArgumentParser(prog='demo.sh', formatter_class=argparse.RawDescriptionHelpFormatter,
        description='Meridian Bank playground helper: list SSO users and teams, inspect virtual servers and print demo credentials.',
        epilog='''examples:
  demo.sh users                          all SSO users with team and role
  demo.sh users --team fraud-strategy    members of one team
  demo.sh user oliver.harrison           one user, incl. initial password
  demo.sh password                       shared initial password (pipe-friendly)
  demo.sh teams | demo.sh team retail-digital-banking
  demo.sh servers --team contact-center-ai
  demo.sh server retail-banking-assistant
  demo.sh credentials                    admin token, Keycloak admin, runtime keys, URLs
  demo.sh key retail-channels-agent      raw runtime key (pipe-friendly)
  demo.sh token oliver.harrison          Keycloak access token for a user (SSO runtime calls)''')
    # Global options are accepted before or after the command; SUPPRESS keeps
    # subcommand defaults from overriding values given before the command.
    common = argparse.ArgumentParser(add_help=False)
    for target, defaults in [(p, dict(context=None, namespace='vmcp-demo', release='vmcp-demo')), (common, None)]:
        default = (lambda key: defaults[key]) if defaults else (lambda _key: argparse.SUPPRESS)
        target.add_argument('--context', default=default('context'), help='Kubernetes context (defaults to the current context)')
        target.add_argument('--namespace', default=default('namespace'), help='quickstart namespace (default: vmcp-demo)')
        target.add_argument('--release', default=default('release'), help='quickstart release prefix (default: vmcp-demo)')
        target.add_argument('--json', action='store_true', default=False if defaults else argparse.SUPPRESS, help='machine-readable JSON output')
    sub = p.add_subparsers(dest='command', metavar='COMMAND')
    add = sub.add_parser
    sub.add_parser = lambda name, **kw: add(name, parents=[common], **kw)
    users = sub.add_parser('users', help='list SSO users (username, name, team, role)')
    users.add_argument('--team', help='only members of this team slug')
    users.add_argument('--passwords', action='store_true', help='include the initial password column')
    sub.add_parser('user', help='show one user with groups, servers and initial password').add_argument('username')
    sub.add_parser('password', help='print the initial SSO password (all demo users share it)').add_argument('username', nargs='?')
    sub.add_parser('teams', help='list teams with members, projects and servers')
    sub.add_parser('team', help='show team members, projects, virtual servers and upstreams').add_argument('slug')
    sub.add_parser('projects', help='list projects and owner teams')
    servers = sub.add_parser('servers', help='list virtual servers')
    servers.add_argument('--team', help='only servers owned by this team slug')
    sub.add_parser('server', help='show a virtual server: endpoint, tools, keys, SSO access').add_argument('slug')
    sub.add_parser('upstreams', help='list mock REST upstreams and their tools')
    sub.add_parser('keys', help='list runtime agent keys with their virtual servers and values')
    sub.add_parser('key', help='print one runtime agent key (pipe-friendly)').add_argument('slug')
    sub.add_parser('credentials', help='admin token, Keycloak credentials, runtime keys and URLs')
    sub.add_parser('urls', help='console, gateway and Keycloak URLs')
    token = sub.add_parser('token', help='fetch a Keycloak access token for a demo user (temporary port-forward)')
    token.add_argument('username')
    token.add_argument('--password-stdin', action='store_true', help='read the password from stdin (if the user changed it)')
    return p


# ------------------------------------------------------------------ cluster
class Cluster:
    def __init__(self, a):
        self.a = a
        self.k = ['kubectl'] + (['--context', a.context] if a.context else []) + ['-n', a.namespace]
        self._secret = None

    def kubectl(self, *args):
        if not shutil.which('kubectl'): raise SystemExit('kubectl is required for this command.')
        result = subprocess.run(self.k + list(args), text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if result.returncode != 0:
            raise SystemExit(f'kubectl {args[0]} {args[1] if len(args) > 1 else ""} failed in namespace {self.a.namespace}. {SECRET_HINT}')
        return result.stdout

    def secret(self):
        if self._secret is None:
            raw = json.loads(self.kubectl('get', 'secret', self.a.release + '-quickstart', '-o', 'json'))
            if raw['metadata'].get('labels', {}).get('vmcp.quickstart') != self.a.release:
                raise SystemExit('Secret is not owned by this quickstart release; refusing to read it.')
            self._secret = {k: base64.b64decode(v).decode() for k, v in raw.get('data', {}).items()}
        return self._secret

    def value(self, key, hint):
        value = self.secret().get(key)
        if not value: raise SystemExit(f'{key} not found in {self.a.release}-quickstart. {hint}')
        return value

    def urls(self):
        services = {self.a.release + '-web': 'console', self.a.release + '-data-plane': 'gateway', self.a.release + '-trial-keycloak': 'keycloak'}
        found = {}
        try:
            ingresses = json.loads(self.kubectl('get', 'ingress', '-o', 'json')).get('items', [])
        except (SystemExit, ValueError):
            ingresses = []
        for ingress in ingresses:
            tls = {h for t in ingress['spec'].get('tls', []) for h in t.get('hosts', [])}
            for rule in ingress['spec'].get('rules', []):
                for path in rule.get('http', {}).get('paths', []):
                    role = services.get(path.get('backend', {}).get('service', {}).get('name'))
                    if role and rule.get('host') and role not in found:
                        found[role] = ('https://' if rule['host'] in tls else 'http://') + rule['host']
        urls = dict(console=found.get('console', 'http://localhost:3300'), gateway=found.get('gateway', 'http://localhost:8801'),
                    keycloak=found.get('keycloak', 'http://localhost:8803'))
        urls['mcpBase'] = urls['gateway'] + '/mcp/v1/virtual/'
        urls['realm'] = urls['keycloak'] + '/realms/' + REALM
        urls['portForward'] = not found
        return urls


# ------------------------------------------------------------------ output
def table(rows, headers):
    rows = [[('' if c is None else str(c)) for c in r] for r in rows]
    widths = [max([len(h)] + [len(r[i]) for r in rows]) for i, h in enumerate(headers)]
    line = lambda cells: '  '.join(c.ljust(w) for c, w in zip(cells, widths)).rstrip()
    print(line(headers))
    print(line(['-' * w for w in widths]))
    for r in rows: print(line(r))


def emit(a, data, render):
    if a.json: print(json.dumps(data, indent=2))
    else: render()


def need(mapping, key, kind):
    if key not in mapping:
        close = [k for k in mapping if key.lower() in k.lower()][:5]
        raise SystemExit(f'Unknown {kind}: {key}.' + (f' Did you mean: {", ".join(close)}?' if close else f' List them with: demo.sh {kind}s'))
    return mapping[key]


def team_name(slug):
    return catalog.TEAM[slug]['name'] if slug in catalog.TEAM else {catalog.ADMIN_GROUP: 'Platform administrators (console admin)',
                                                                   catalog.VIEWER_GROUP: 'Auditors (read-only console)'}.get(slug, slug)


def user_row(u):
    return dict(username=u['username'], name=u['firstName'] + ' ' + u['lastName'], email=u['email'], team=u['team'],
                groups=u['groups'], title=u['title'])


def server_row(s):
    return dict(slug=s['slug'], name=s['name'], project=s['project'], team=catalog.server_team(s), description=s['description'],
                tools=[dict(tool=t, upstream=demo_upstreams.TOOL_UPSTREAM[t]) for t in s['tools']],
                agentKeys=catalog.server_keys(s['slug']), ssoGroup=catalog.server_team(s), instructions=s['instructions'])


USERS = {u['username']: u for u in catalog.USERS}


# ------------------------------------------------------------------ commands
def cmd_users(a, c):
    users = catalog.USERS
    if a.team:
        need(catalog.TEAM, a.team, 'team')
        users = catalog.team_members(a.team)
    password = c.value('keycloak-demo-password', 'Install with --keycloak.') if a.passwords else None
    data = [dict(user_row(u), **({'initialPassword': password} if password else {})) for u in users]
    def render():
        table([[u['username'], u['name'], u['team'] or u['groups'][0], u['title']] + ([password] if password else []) for u in data],
              ['USERNAME', 'NAME', 'TEAM / GROUP', 'ROLE'] + (['INITIAL PASSWORD'] if password else []))
        print(f'\n{len(data)} users. Sign in to the console with username + shared initial password: demo.sh password')
    emit(a, data, render)


def cmd_user(a, c):
    u = need(USERS, a.username, 'user')
    password = c.value('keycloak-demo-password', 'Install with --keycloak.')
    data = dict(user_row(u), servers=catalog.team_servers(u['team']) if u['team'] else [], initialPassword=password,
                consoleRole='admin' if catalog.ADMIN_GROUP in u['groups'] else 'viewer' if catalog.VIEWER_GROUP in u['groups'] else 'team member (' + u['team'] + ')')
    def render():
        for label, key in [('Username', 'username'), ('Name', 'name'), ('Email', 'email'), ('Role', 'title'), ('Console', 'consoleRole')]:
            print(f'{label + ":":<18}{data[key]}')
        print(f'{"Team:":<18}{team_name(u["team"] or u["groups"][0])} ({", ".join(u["groups"])})')
        if data['servers']: print(f'{"SSO servers:":<18}{", ".join(data["servers"])}')
        print(f'{"Password:":<18}{password}   (initial; changes made in Keycloak are preserved)')
    emit(a, data, render)


def cmd_password(a, c):
    if a.username: need(USERS, a.username, 'user')
    password = c.value('keycloak-demo-password', 'Install with --keycloak.')
    emit(a, {'username': a.username, 'initialPassword': password}, lambda: print(password))


def cmd_teams(a, _c):
    data = [dict(slug=t['slug'], name=t['name'], mission=t['mission'], members=[u['username'] for u in catalog.team_members(t['slug'])],
                 projects=[p['slug'] for p in catalog.PROJECT.values() if p['ownerTeam'] == t['slug']], servers=catalog.team_servers(t['slug']),
                 upstreams=[u['slug'] for u in demo_upstreams.UPSTREAMS if u['ownerTeam'] == t['slug']]) for t in catalog.TEAM.values()]
    emit(a, data, lambda: table([[t['slug'], t['name'], len(t['members']), len(t['projects']), len(t['servers']), len(t['upstreams'])] for t in data],
                                ['TEAM (SSO GROUP)', 'NAME', 'MEMBERS', 'PROJECTS', 'SERVERS', 'UPSTREAMS']))


def cmd_team(a, _c):
    t = need(catalog.TEAM, a.slug, 'team')
    members = [user_row(u) for u in catalog.team_members(t['slug'])]
    projects = [p for p in catalog.PROJECT.values() if p['ownerTeam'] == t['slug']]
    servers = [catalog.SERVER[s] for s in catalog.team_servers(t['slug'])]
    upstreams = [u for u in demo_upstreams.UPSTREAMS if u['ownerTeam'] == t['slug']]
    data = dict(t, members=members, projects=projects, servers=[s['slug'] for s in servers], upstreams=[u['slug'] for u in upstreams],
                identityBinding=catalog.identity_binding_slug(t['slug']) if servers else None)
    def render():
        print(f'{t["name"]} ({t["slug"]})\n{t["mission"]}\n')
        table([[m['username'], m['name'], m['title'], m['email']] for m in members], ['MEMBER', 'NAME', 'ROLE', 'EMAIL'])
        print()
        for p in projects: print(f'Project  {p["slug"]:<36}{p["purpose"]}')
        for s in servers: print(f'Server   {s["slug"]:<36}{len(s["tools"])} tools, project {s["project"]}')
        for u in upstreams: print(f'Upstream {u["slug"]:<36}{len(u["operations"])} tools')
        if data['identityBinding']:
            print(f'\nMembers reach the team servers with their SSO token (identity binding {data["identityBinding"]}): demo.sh token {members[0]["username"]}')
    emit(a, data, render)


def cmd_projects(a, _c):
    data = [dict(p, servers=[s['slug'] for s in catalog.SERVERS if s['project'] == p['slug']]) for p in catalog.PROJECT.values()]
    emit(a, data, lambda: table([[p['slug'], p['name'], p['ownerTeam'], ', '.join(p['servers'])] for p in data],
                                ['PROJECT', 'NAME', 'OWNER TEAM', 'VIRTUAL SERVERS']))


def cmd_servers(a, _c):
    servers = catalog.SERVERS
    if a.team:
        need(catalog.TEAM, a.team, 'team')
        servers = [catalog.SERVER[s] for s in catalog.team_servers(a.team)]
    data = [server_row(s) for s in servers]
    emit(a, data, lambda: table([[s['slug'], catalog.server_team(catalog.SERVER[s['slug']]), len(s['tools']), ', '.join(s['agentKeys'])] for s in data],
                                ['VIRTUAL SERVER', 'OWNER TEAM', 'TOOLS', 'AGENT KEY']))


def cmd_server(a, c):
    s = need(catalog.SERVER, a.slug, 'server')
    data = server_row(s)
    data['endpoint'] = c.urls()['mcpBase'] + s['slug']
    data['pendingOrRejectedGrants'] = s.get('grants', {})
    def render():
        print(f'{s["name"]} ({s["slug"]})\n{s["description"]}\n')
        print(f'{"Endpoint:":<14}{data["endpoint"]}   (Streamable HTTP)')
        print(f'{"Project:":<14}{s["project"]} - team {data["team"]}')
        print(f'{"Agent keys:":<14}{", ".join(data["agentKeys"])}   -> demo.sh key {data["agentKeys"][0]}')
        print(f'{"SSO access:":<14}members of group {data["ssoGroup"]} -> demo.sh users --team {data["ssoGroup"]}')
        print(f'{"Instructions:":<14}{s["instructions"]}\n')
        table([[t['tool'], t['upstream'], s.get('grants', {}).get(t['tool'], '')] for t in data['tools']], ['TOOL', 'UPSTREAM', 'GRANT'])
    emit(a, data, render)


def cmd_upstreams(a, _c):
    data = [dict(slug=u['slug'], name=u['name'], ownerTeam=u['ownerTeam'], description=u['description'],
                 tools=[dict(name=o['name'], method=o['method'], readOnly=o['readOnly'], description=o['description']) for o in u['operations']])
            for u in demo_upstreams.UPSTREAMS]
    emit(a, data, lambda: table([[u['slug'], u['ownerTeam'], ', '.join(t['name'] for t in u['tools'])] for u in data],
                                ['UPSTREAM', 'OWNER TEAM', 'TOOLS']))


def agent_keys(c):
    stored = c.secret()
    return [dict(slug=slug, description=desc, servers=servers, key=stored.get(slug)) for slug, desc, servers in catalog.AGENT_KEYS]


def cmd_keys(a, c):
    data = agent_keys(c)
    if not any(k['key'] for k in data): raise SystemExit('No runtime keys stored. Install with --mock.')
    def render():
        for k in data:
            print(f'{k["slug"]}  ({k["description"]})\n  key:     {k["key"] or "(not created)"}\n  servers: {", ".join(k["servers"])}')
        print('\nMCP clients: Authorization: Bearer <key>, URL <gateway>/mcp/v1/virtual/<server>  (demo.sh urls)')
    emit(a, data, render)


def cmd_key(a, c):
    slug = need({k[0]: k for k in catalog.AGENT_KEYS}, a.slug, 'key')[0]
    key = c.value(slug, 'Install with --mock.')
    emit(a, {'slug': slug, 'key': key}, lambda: print(key))


def cmd_urls(a, c):
    urls = c.urls()
    def render():
        print(f'Console:   {urls["console"]}\nGateway:   {urls["mcpBase"]}<server>\nKeycloak:  {urls["keycloak"]}  (realm {REALM})')
        if urls['portForward']:
            print('No ingress found: these are the quickstart port-forward addresses (keep quickstart.sh running or rerun it).')
    emit(a, urls, render)


def cmd_credentials(a, c):
    stored, urls = c.secret(), c.urls()
    keys = [k for k in agent_keys(c) if k['key']]
    data = dict(urls=urls, adminToken=stored.get('admin-token'), keycloak=None, agentKeys=keys)
    if stored.get('keycloak-demo-password'):
        data['keycloak'] = dict(adminConsole=urls['keycloak'] + '/admin/', adminUser='admin', adminPassword=stored.get('keycloak-admin-password'),
                                realm=REALM, demoUserPassword=stored['keycloak-demo-password'], users=len(catalog.USERS))
    def render():
        print(f'Console          {urls["console"]}\nGateway          {urls["mcpBase"]}<server>')
        print(f'Admin token      {data["adminToken"]}   (console "token" sign-in)')
        if data['keycloak']:
            k = data['keycloak']
            print(f'\nKeycloak         {k["adminConsole"]}  user admin / {k["adminPassword"]}')
            print(f'SSO users        {k["users"]} users in realm {REALM}, initial password {k["demoUserPassword"]}')
            print('                 e.g. platform.admin (admin), security.auditor (viewer), oliver.harrison (team) - demo.sh users')
        if keys:
            print()
            table([[k['slug'], k['key'], len(k['servers'])] for k in keys], ['AGENT KEY', 'VALUE', 'SERVERS'])
        if urls['portForward']: print('\nURLs are quickstart port-forward addresses (no ingress found).')
    emit(a, data, render)


def cmd_token(a, c):
    need(USERS, a.username, 'user')
    import playground
    password = sys.stdin.readline().rstrip('\n') if a.password_stdin else c.value('keycloak-demo-password', 'Install with --keycloak.')
    body = dict(client_id=AGENT_CLIENT, grant_type='password', username=a.username, password=password, scope='openid')
    path = f'/realms/{REALM}/protocol/openid-connect/token'
    try:
        with playground.forward(c, a.release + '-trial-keycloak') as endpoint:
            result = playground.API(endpoint).request('POST', path, body, form=True)
    except ValueError as exc:
        raise SystemExit(str(exc).replace('Demo API', 'Keycloak').split(';')[0] + '. Check the username/password (users may have changed it).') from None
    emit(a, {k: result.get(k) for k in ['access_token', 'expires_in', 'token_type']}, lambda: print(result['access_token']))


COMMANDS = {name[4:]: fn for name, fn in globals().items() if name.startswith('cmd_')}


def main(argv=None):
    a = parser().parse_args(argv)
    if not a.command:
        parser().print_help()
        return
    COMMANDS[a.command](a, Cluster(a))


if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(130)
    except BrokenPipeError:
        sys.exit(0)
