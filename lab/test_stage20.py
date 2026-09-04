import unittest

import stage20


class Stage20ControlTests(unittest.TestCase):
    def test_parse_time_accepts_go_trimmed_and_nanosecond_rfc3339(self):
        expected='2026-09-04T19:17:13.576750+00:00'
        for value in (
            '2026-09-04T19:17:13.57675Z',
            '2026-09-04T19:17:13.576750Z',
            '2026-09-04T19:17:13.576750649Z',
            '2026-09-04T19:17:13.57675+00:00',
        ):
            with self.subTest(value=value):
                self.assertEqual(stage20.parse_time(value).isoformat(),expected)

    def test_parse_time_accepts_whole_seconds(self):
        self.assertEqual(stage20.parse_time('2026-09-04T19:17:13Z').isoformat(),
                         '2026-09-04T19:17:13+00:00')


if __name__ == '__main__':
    unittest.main()
