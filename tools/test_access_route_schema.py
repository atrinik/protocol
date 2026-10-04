"""Exercise the published decimal uint64 schema against independent boundaries."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parent.parent


class AccessRouteSchemaTests(unittest.TestCase):
    def test_token_revision_bounds(self):
        schema = json.loads((ROOT / 'schema/access-route-v1.schema.json').read_text())
        rule = schema['properties']['tokenRevision']
        vectors = json.loads((ROOT / 'fixtures/access-route-bounds-v1.json').read_text())
        maximum = (1 << 64) - 1

        def accepts(value):
            return (rule['minLength'] <= len(value) <= rule['maxLength']
                    and re.search(rule['pattern'], value) is not None
                    and re.search(rule['not']['pattern'], value) is None)

        for value in vectors['valid']:
            with self.subTest(valid=value):
                self.assertTrue(accepts(value))
        for value in vectors['invalid']:
            with self.subTest(invalid=value):
                self.assertFalse(accepts(value))
        # Every decimal prefix divergence near the maximum and each power of ten.
        values = {maximum + offset for offset in range(-100, 101)}
        for power in range(21):
            values.update(10 ** power + offset for offset in (-1, 0, 1))
        for width in range(1, 21):
            scale = 10 ** width
            boundary = (maximum // scale) * scale
            values.update(boundary + offset for offset in (-1, 0, 1, scale - 1, scale))
        for value in values:
            with self.subTest(number=value):
                self.assertEqual(accepts(str(value)), 1 <= value <= maximum)


if __name__ == '__main__':
    unittest.main()
