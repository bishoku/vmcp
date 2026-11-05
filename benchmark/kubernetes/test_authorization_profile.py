import unittest
from authorization_profile import measure
from authorization_acceptance import LoginForm


class AuthorizationMeasurementTests(unittest.TestCase):
    def test_rejected_responses_cannot_pass_as_fast_successes(self):
        result = measure(lambda: False, duration=.01, concurrency=2)
        self.assertFalse(result['passed'])
        self.assertEqual(result['successful'], 0)
        self.assertGreater(result['errors'], 0)
        self.assertIsNone(result['p95Ms'])
        self.assertEqual(result['successfulRequestsPerSecond'], 0)

    def test_transport_errors_are_counted(self):
        def failed():
            raise ConnectionError('synthetic disconnect')
        result = measure(failed, duration=.01, concurrency=2)
        self.assertFalse(result['passed'])
        self.assertGreater(result['errors'], 0)

    def test_success_counts_have_ordered_percentiles(self):
        result = measure(lambda: True, duration=.01, concurrency=2)
        self.assertTrue(result['passed'])
        self.assertGreater(result['successful'], 0)
        self.assertLessEqual(result['p50Ms'], result['p95Ms'])
        self.assertLessEqual(result['p95Ms'], result['p99Ms'])

    def test_real_idp_form_action_decodes_query_entities(self):
        form = LoginForm()
        form.feed('<form id="unrelated" action="/wrong"></form><form id="kc-form-login" action="http://127.0.0.1/login?session=x&amp;execution=y"></form>')
        self.assertEqual(form.action, 'http://127.0.0.1/login?session=x&execution=y')


if __name__ == '__main__':
    unittest.main()
