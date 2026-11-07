"""Keycloak realm for actual conditional LoA + OTP (no hardcoded assurance mapper)."""
def execution(authenticator=None, flow=None, requirement='REQUIRED', priority=0, config=None):
    e = dict(requirement=requirement, priority=priority, authenticatorFlow=bool(flow))
    if flow: e['flowAlias'] = flow
    if authenticator: e['authenticator'] = authenticator
    if config: e['authenticatorConfig'] = config
    return e


def realm(origin='https://localhost:8844'):
    flows = [dict(alias='demo-step-up', providerId='basic-flow', topLevel=True, builtIn=False,
                  authenticationExecutions=[execution('auth-cookie', requirement='ALTERNATIVE'),
                    execution(flow='demo-levels', requirement='ALTERNATIVE', priority=10)]),
             dict(alias='demo-levels', providerId='basic-flow', topLevel=False, builtIn=False,
                  authenticationExecutions=[execution(flow='demo-password', requirement='CONDITIONAL'),
                    execution(flow='demo-otp', requirement='CONDITIONAL', priority=10)]),
             dict(alias='demo-password', providerId='basic-flow', topLevel=False, builtIn=False,
                  authenticationExecutions=[execution('conditional-level-of-authentication', config='demo-loa-1'),
                    execution('auth-username-password-form', priority=10)]),
             dict(alias='demo-otp', providerId='basic-flow', topLevel=False, builtIn=False,
                  authenticationExecutions=[execution('conditional-level-of-authentication', config='demo-loa-2'),
                    execution('auth-otp-form', priority=10)])]
    mappers = [dict(name='runtime-audience', protocol='openid-connect', protocolMapper='oidc-audience-mapper',
                   config={'included.custom.audience':'vmcp-runtime','access.token.claim':'true','id.token.claim':'false'}),
               dict(name='acr',protocol='openid-connect',protocolMapper='oidc-acr-mapper',config={'access.token.claim':'true','id.token.claim':'true'})]
    result = dict(realm='vmcp-security', enabled=True, sslRequired='none', registrationAllowed=False,
                browserFlow='demo-step-up', authenticationFlows=flows,
                authenticatorConfig=[dict(alias='demo-loa-1', config={'loa-condition-level':'1','loa-max-age':'36000'}),
                                     dict(alias='demo-loa-2', config={'loa-condition-level':'2','loa-max-age':'0'})],
                clients=[dict(clientId='vmcp-security-demo', protocol='openid-connect', enabled=True, publicClient=True,
                              standardFlowEnabled=True, directAccessGrantsEnabled=False,
                              redirectUris=[origin+'/',origin+'/index.html',origin+'/approve.html'],webOrigins=[origin],
                              attributes={'pkce.code.challenge.method':'S256','acr.loa.map':'{"1":1,"2":2}'},protocolMappers=mappers)],
                users=[dict(username=name,enabled=True,emailVerified=True,firstName=name.title(),lastName='Demo',email=name+'@example.test',
                            credentials=[dict(type='password',value='demo-password',temporary=False)]) for name in ['alice','bob']])

    import copy
    approval=copy.deepcopy(result['clients'][0])
    approval['clientId']='vmcp-approval-demo'
    approval['redirectUris']=[origin+'/approve.html']
    approval['protocolMappers'][0]['config']['included.custom.audience']='vmcp-approval'
    result['clients'][0]['redirectUris']=[origin+'/',origin+'/index.html']
    result['clients'].append(approval)
    customer=copy.deepcopy(result['clients'][0])
    customer['clientId']='vmcp-customer-demo'
    customer['redirectUris']=[origin+'/',origin+'/index.html',origin+'/session.html']
    customer['protocolMappers'][0]['config']['included.custom.audience']='vmcp-customer'
    result['clients'].append(customer)
    return result
