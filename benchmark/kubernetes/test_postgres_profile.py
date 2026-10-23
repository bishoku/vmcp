import unittest
from postgres_profile import arguments, settings

class PostgresProfileContract(unittest.TestCase):
    def test_tuning_keeps_wal_durability_and_bounded_memory(self):
        profile=settings(True)
        for key in ('fsync','synchronous_commit','full_page_writes'):
            self.assertEqual(profile[key],'on')
        self.assertEqual(profile['max_connections'],'256')
        self.assertEqual(profile['work_mem'],'4MB')
        self.assertEqual(profile['shared_buffers'],'1GB')
        self.assertNotIn('enable_seqscan',profile)
        self.assertNotIn('wal_level',profile)
    def test_baseline_is_not_silently_tuned(self):
        self.assertNotIn('shared_buffers',settings(False))
        self.assertNotIn('synchronous_commit=off',arguments(True))

if __name__=='__main__':unittest.main()
