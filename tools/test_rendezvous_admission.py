"""Independent, bounded rendezvous admission byte-contract conformance checks."""
import hashlib
import hmac
import json
from pathlib import Path
import re
import unittest

FIXTURE = Path(__file__).resolve().parents[1] / 'fixtures/rendezvous-admission-v1.json'
SCHEMA = 'atrinik-rendezvous-admission-v1'
HEX64 = re.compile(r'[0-9a-f]{64}', re.ASCII)
TOKEN = re.compile(
    r'v1\.([A-Za-z0-9_-]{1,32})\.([1-9][0-9]{0,11})\.'
    r'([0-9a-f]{64})\.([0-9a-f]{32})\.([0-9a-f]{64})', re.ASCII)
MAX_SECONDS = 253402300799


def verify(admission, ticket, authority, server_id, now, current, previous):
    """Pure reference verifier; does not invoke implementation or mine work."""
    if not isinstance(admission, str) or len(admission) > 256:
        return False
    if not HEX64.fullmatch(ticket) or not HEX64.fullmatch(server_id):
        return False
    token, separator, solution = admission.rpartition('.')
    if not separator or not re.fullmatch(r'[0-9a-f]{16}', solution, re.ASCII):
        return False
    parsed = TOKEN.fullmatch(token)
    if parsed is None:
        return False
    key_id, issued, ticket_hash, _, mac = parsed.groups()
    issued = int(issued)
    if issued + 60 > MAX_SECONDS or now < issued or now >= issued + 60:
        return False
    if not hmac.compare_digest(ticket_hash, hashlib.sha256(bytes.fromhex(ticket)).hexdigest()):
        return False
    key = current.get(key_id, previous.get(key_id))
    if key is None:
        return False
    prefix = token.rsplit('.', 1)[0]
    signing = (b'atrinik-rendezvous-admission-v1\0' + authority.encode('ascii')
               + b'\0classic\0' + server_id.encode('ascii') + b'\0' + prefix.encode('ascii'))
    if not hmac.compare_digest(mac, hmac.new(key, signing, hashlib.sha256).hexdigest()):
        return False
    work = b'atrinik-rendezvous-work-v1\0' + token.encode('ascii') + b'\0' + solution.encode('ascii')
    return hashlib.sha256(work).digest()[:3] == b'\0\0\0'


def canonical_json(raw, limit, fields):
    if len(raw) > limit:
        raise ValueError('bound')
    def pairs(items):
        if [key for key, _ in items] != fields:
            raise ValueError('keys')
        return dict(items)
    value = json.loads(raw.decode('utf-8'), object_pairs_hook=pairs)
    if not isinstance(value, dict) or json.dumps(value, separators=(',', ':')).encode() != raw:
        raise ValueError('canonical')
    if value['schema'] != SCHEMA:
        raise ValueError('schema')
    return value


class AdmissionFixtures(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.f = json.loads(FIXTURE.read_text())

    def test_independent_bytes(self):
        f = self.f
        key = bytes.fromhex(f['keyHex'])
        self.assertEqual(len(key), 32)
        self.assertTrue(f['synthetic'])
        self.assertEqual(hashlib.sha256(bytes.fromhex(f['ticket'])).hexdigest(), f['ticketHash'])
        self.assertNotEqual(hashlib.sha256(f['ticket'].encode()).hexdigest(), f['ticketHash'])
        signing = (b'atrinik-rendezvous-admission-v1\0' + f['authority'].encode()
                   + b'\0classic\0' + f['serverId'].encode() + b'\0' + f['tokenPrefix'].encode())
        self.assertEqual(signing.hex(), f['hmacInputHex'])
        self.assertEqual(hmac.new(key, signing, hashlib.sha256).hexdigest(), f['mac'])
        self.assertEqual(f['tokenPrefix'] + '.' + f['mac'], f['token'])
        work = b'atrinik-rendezvous-work-v1\0' + f['token'].encode() + b'\0' + f['solution'].encode()
        self.assertEqual(work.hex(), f['workInputHex'])
        self.assertEqual(hashlib.sha256(work).hexdigest(), f['workDigest'])
        self.assertTrue(f['workDigest'].startswith('000000'))
        self.assertEqual(f['difficulty'], 24)
        self.assertEqual(str(int(f['issuedAt']) + 60), f['expiresAt'])

    def test_positive_and_negative_vectors(self):
        f = self.f
        for case in f['cases']:
            with self.subTest(case=case['name']):
                value = {**f, **case.get('override', {})}
                key = bytes.fromhex(f['keyHex'])
                role = case.get('keyRole', 'current')
                current = {f['keyId']: key} if role == 'current' else {'synthetic_rotated': bytes(range(32, 64))}
                previous = {f['keyId']: key} if role == 'previous' else {}
                accepted = verify(value['token'] + '.' + value['solution'], value['ticket'],
                                  value['authority'], value['serverId'], case['now'], current, previous)
                self.assertEqual(accepted, case['accepted'])

    def test_exact_challenge_request(self):
        f = self.f
        valid = ('{"schema":"' + SCHEMA + '","ticketHash":"' + f['ticketHash'] + '"}').encode()
        fields = ['schema', 'ticketHash']
        self.assertEqual(canonical_json(valid, 256, fields)['ticketHash'], f['ticketHash'])
        invalid = [valid + b'\n', valid.replace(b',', b', '), valid[:-1] + b',"extra":0}',
                   valid.replace(b'"ticketHash":', b'"schema":"x","ticketHash":'),
                   b'[]', b' ' * 257, valid.replace(b'"schema"', b'"ticketHash"', 1)]
        for raw in invalid:
            with self.subTest(raw=raw[:100]), self.assertRaises(ValueError):
                canonical_json(raw, 256, fields)

    def test_exact_challenge_response(self):
        f = self.f
        valid = {'schema': SCHEMA, 'challenge': f['token'], 'difficulty': 24, 'expiresAt': f['expiresAt']}
        def parse(raw):
            result = canonical_json(raw, 512, ['schema', 'challenge', 'difficulty', 'expiresAt'])
            if type(result['difficulty']) is not int or result['difficulty'] != 24:
                raise ValueError('difficulty')
            matched = TOKEN.fullmatch(result['challenge'])
            if matched is None or str(int(matched[2]) + 60) != result['expiresAt']:
                raise ValueError('expiry')
            return result
        self.assertEqual(parse(json.dumps(valid, separators=(',', ':')).encode()), valid)
        for field, value in [('difficulty', 0), ('difficulty', 25), ('difficulty', True),
                             ('difficulty', '24'), ('expiresAt', '1800000061'),
                             ('expiresAt', 1800000060), ('challenge', f['token'].upper())]:
            with self.subTest(field=field, value=value), self.assertRaises(ValueError):
                parse(json.dumps({**valid, field: value}, separators=(',', ':')).encode())
        with self.assertRaises(ValueError):
            parse(b' ' * 513)


if __name__ == '__main__':
    unittest.main()
