import copy
import importlib.util
import json
import pathlib
import tempfile
import unittest

ROOT=pathlib.Path(__file__).resolve().parents[2]
def module(name,path):
    spec=importlib.util.spec_from_file_location(name,path)
    result=importlib.util.module_from_spec(spec);spec.loader.exec_module(result)
    return result
reports=module('acceptance_report',ROOT/'benchmark/acceptance/report.py')
comparison=module('kube_compare',ROOT/'benchmark/kubernetes/compare.py')

class ReportContract(unittest.TestCase):
    def test_kubernetes_report_does_not_claim_native_sigkill_or_compose_limits(self):
        data=json.loads((ROOT/'benchmark/results/rest-release-20261020/after-fix/report.json').read_text())
        data=copy.deepcopy(data)
        data['DataPlaneConfiguration']['platform']='MicroK8s Kubernetes'
        with tempfile.TemporaryDirectory() as directory:
            source=pathlib.Path(directory)/'report.json';source.write_text(json.dumps(data))
            text=reports.render(source).read_text()
            self.assertIn('MicroK8s üzerinde',text)
            self.assertIn('fiziksel Kubernetes node arızası',text)
            self.assertIn('Kubernetes CPU limiti 4',text)
            self.assertNotIn('Node kaybı SIGKILL ile yapılır',text)
            self.assertNotIn('PostgreSQL 17 konteyneri 2 CPU / 2 GiB',text)
            self.assertIn('python3 benchmark/kubernetes/run.py',text)
    def test_database_focus_reports_unlimited_postgres_without_fault_claim(self):
        data=json.loads((ROOT/'benchmark/results/rest-release-20261020/after-fix/report.json').read_text())
        data['DataPlaneConfiguration'].update(platform='MicroK8s Kubernetes', database_focus=True, postgres_limits='none', steady_seconds=600, nodes=1, go_soft_memory_limit_each='4GiB', memory_limit_each='12Gi')
        next(p for p in data['Phases'] if p['name']=='steady')['target_rps']=1200
        with tempfile.TemporaryDirectory() as directory:
            source=pathlib.Path(directory)/'report.json';source.write_text(json.dumps(data))
            text=reports.render(source).read_text()
            self.assertIn('PostgreSQL CPU/bellek limiti yok',text)
            self.assertIn('600 saniye 1200 TPS',text)
            self.assertIn('GOMEMLIMIT=4GiB',text)
            self.assertIn('bellek limiti 12GiB',text)
            self.assertIn('tek data-plane podu',text)
            self.assertIn('pod kapatma veya konfigürasyon mutasyonu yapılmaz',text)
            self.assertIn('run.py --database-focus',text)
            self.assertNotIn('PostgreSQL limiti 4 CPU',text)
    def test_refresh_timeout_is_not_presented_as_measured_propagation(self):
        data=json.loads((ROOT/'benchmark/results/rest-release-20261020/after-fix/report.json').read_text())
        data['Functional']['configuration_refresh_under_rest_load']=False
        data['DataPlaneConfiguration']['rest_refresh_ms']=25000
        with tempfile.TemporaryDirectory() as directory:
            source=pathlib.Path(directory)/'report.json';source.write_text(json.dumps(data))
            text=reports.render(source).read_text()
            self.assertIn('25000 ms gözlem penceresinde doğrulanamadı',text)
            self.assertNotIn('snapshot yayılımı: **25000 ms**',text)
    def test_smoke_is_not_reported_as_capacity_comparison(self):
        with tempfile.TemporaryDirectory() as directory:
            (pathlib.Path(directory)/'report.json').write_text(json.dumps({'Profile':{'Smoke':True}}))
            text=comparison.render(directory).read_text()
            self.assertIn('kapasite karşılaştırması değildir',text)
            self.assertNotIn('60.000 tool',text)

if __name__=='__main__':unittest.main()
