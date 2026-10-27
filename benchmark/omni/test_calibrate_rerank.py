import copy
import unittest
from calibrate_rerank import calibrate, metrics


def candidate(name, score):
    return dict(name=name, tool_ref=name, rerank_score=score, retrieval_score=0.5)


def row(split, language, expected, scores):
    return dict(split=split, case=dict(language=language, expected=expected, query='synthetic'), candidates=scores)


class CalibrationTests(unittest.TestCase):
    def fixture(self):
        rows = []
        for split in ('calibration', 'validation'):
            for lang in ('en', 'tr'):
                rows.append(row(split, lang, ['correct'], [candidate('correct', .03), candidate('wrong', .05)]))
            rows.append(row(split, 'en', [], [candidate('unrelated', .00002)]))
        return dict(results=rows, embedding_model='synthetic', rerank_model='synthetic')

    def test_validation_labels_cannot_select_thresholds(self):
        report = self.fixture()
        before = calibrate(report)
        changed = copy.deepcopy(report)
        for row in changed['results']:
            if row['split'] == 'validation' and row['case']['expected']:
                row['case']['expected'] = ['unknown']
        after = calibrate(changed)
        self.assertTrue(before['calibration_candidate_found'])
        self.assertEqual(before['selected_no_match_score'], after['selected_no_match_score'])
        self.assertEqual(before['selected_confidence_score'], after['selected_confidence_score'])
        self.assertTrue(before['validation_non_regression_gate'])
        scored = copy.deepcopy(report)
        for row in scored['results']:
            if row['split'] == 'validation' and row['case']['expected']:
                row['candidates'][1]['rerank_score'] = .9
        altered = calibrate(scored)
        self.assertEqual(before['selected_confidence_score'], altered['selected_confidence_score'])
        self.assertFalse(altered['validation_non_regression_gate'])

    def test_query_gate_and_uncertainty_are_separate(self):
        rows = self.fixture()['results']
        measured = metrics(rows, 'validation', .0001, .1)
        self.assertEqual(measured['negative']['false_positive_queries'], 0)
        self.assertEqual(measured['en']['hit1'], 1)
        self.assertEqual(measured['tr']['hit1'], 1)
        aggressive = metrics(rows, 'validation', .0001, 0)
        self.assertEqual(aggressive['en']['hit1'], 0)
        self.assertEqual(aggressive['en']['hit5'], 1)


if __name__ == '__main__':
    unittest.main()
