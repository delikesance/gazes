import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("bootstrap", Path(__file__).with_name("prowlarr-bootstrap.py"))
b = importlib.util.module_from_spec(spec)
spec.loader.exec_module(b)


class FakeAPI:
    def __init__(self):
        self.schemas = [{"implementation": "Anidex", "fields": []},
                        {"implementation": "Cardigann", "definitionName": "thepiratebay", "fields": []}]
        self.existing = []
        self.creates = 0

    def __call__(self, path, payload=None, method=None):
        if path == "indexer/schema": return self.schemas
        if path == "indexer": return self.existing
        if path == "appprofile": return [{"id": 1}]
        if method == "PUT":
            self.existing[:] = [payload if i["id"] == payload["id"] else i for i in self.existing]
            return payload
        assert path == "indexer?forceSave=true"
        assert payload["enable"] is False
        self.creates += 1
        return dict(payload, id=self.creates)


class BootstrapTests(unittest.TestCase):
    def test_initialization_keeps_key_and_manual_auth(self):
        with tempfile.TemporaryDirectory() as directory:
            path = b.initialize(directory)
            first = path.read_bytes()
            self.assertEqual(len(b.ET.parse(path).getroot().findtext("ApiKey")), 32)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            path.write_bytes(first.replace(b"External", b"Forms"))
            expected = path.read_bytes()
            b.initialize(directory)
            self.assertEqual(path.read_bytes(), expected)

    def test_restart_does_not_duplicate(self):
        api = FakeAPI()
        first = b.provision(api, "http://prowlarr:9696", "secret")
        self.assertEqual(len(first), 2)
        self.assertEqual(b.provision(api, "http://prowlarr:9696", "secret"), first)
        self.assertEqual(api.creates, 2)
        self.assertTrue(first[0]["endpoint"].endswith("/1/api"))

    def test_manual_configuration_preserved(self):
        api = FakeAPI()
        manual = {"implementation": "Anidex", "id": 42, "enable": False, "name": "Personal", "fields": [{"value": "custom"}]}
        api.existing = [manual.copy(), {"implementation": "Other", "id": 9}]
        result = b.provision(api, "http://prowlarr:9696", "secret")
        self.assertEqual(api.existing[0], manual)
        self.assertEqual(len(result), 1)
        self.assertEqual(api.creates, 1)
        self.assertEqual(api.existing[1]["id"], 9)

    def test_interrupted_activation_resumes_without_duplicates(self):
        api = FakeAPI()
        original = api.__call__
        def fail_put(path, payload=None, method=None):
            if method == "PUT": raise b.APIError("unavailable")
            return original(path, payload, method)
        with tempfile.TemporaryDirectory() as directory:
            pending = str(Path(directory) / "pending.json")
            with self.assertRaises(b.APIError):
                b.provision(fail_put, "http://prowlarr:9696", "secret", pending)
            self.assertEqual(api.creates, 1)
            self.assertFalse(api.existing[0]["enable"])
            result = b.provision(api, "http://prowlarr:9696", "secret", pending)
            self.assertEqual(len(result), 2)
            self.assertEqual(api.creates, 2)
            self.assertEqual(json.loads(Path(pending).read_text()), [])

    def test_missing_definitions_skip(self):
        api = FakeAPI()
        api.schemas = []
        self.assertEqual(b.provision(api, "http://prowlarr:9696", "secret"), [])
        self.assertEqual(api.creates, 0)

    def test_enabled_extra_torrent_gateways_are_exported(self):
        api = FakeAPI()
        api.existing = [
            {"implementation": "Cardigann", "definitionName": "ext", "id": 31, "enable": True, "protocol": "torrent"},
            {"implementation": "Other", "id": 32, "enable": True, "protocol": "torrent"},
            {"implementation": "Other", "id": 33, "enable": False, "protocol": "torrent"},
            {"implementation": "Other", "id": 34, "enable": True, "protocol": "usenet"},
        ]
        original = [dict(row) for row in api.existing]
        result = b.provision(api, "http://prowlarr:9696", "secret")
        self.assertEqual({r["name"] for r in result}, {"anidex", "thepiratebay", "ext", "prowlarr-32"})
        self.assertEqual(api.existing[:4], original)
        self.assertEqual(b.provision(api, "http://prowlarr:9696", "secret"), result)

    def test_ext_is_routed_through_flaresolverr_once(self):
        api = FakeAPI()
        api.schemas.append({"implementation": "Cardigann", "definitionName": "ext", "fields": []})
        calls = []
        base = api.__call__
        proxies, tags = [], []
        def routed(path, payload=None, method=None):
            calls.append(path)
            if path == "tag":
                if payload is None: return tags
                tags.append(dict(payload, id=7)); return tags[-1]
            if path == "indexerProxy": return proxies
            if path == "indexerProxy/schema":
                return [{"implementation": "FlareSolverr", "fields": [{"name": "host", "value": ""}]}]
            if path == "indexerProxy?forceSave=true":
                proxies.append(payload); return payload
            return base(path, payload, method)
        first = b.provision(routed, "http://prowlarr:9696", "secret", None, "http://flaresolverr:8191")
        self.assertEqual({r["name"] for r in first}, {"anidex", "thepiratebay", "ext"})
        self.assertEqual(proxies[0]["fields"][0]["value"], "http://flaresolverr:8191")
        self.assertEqual(proxies[0]["tags"], [7])
        ext = next(i for i in api.existing if i.get("definitionName") == "ext")
        self.assertEqual(ext["tags"], [7])
        b.provision(routed, "http://prowlarr:9696", "secret", None, "http://flaresolverr:8191")
        self.assertEqual(len(proxies), 1)
        self.assertEqual(len(tags), 1)

    def test_account_tracker_needs_its_key_and_keeps_manual_setup(self):
        api = FakeAPI()
        api.schemas.append({"implementation": "Cardigann", "definitionName": "c411",
                            "fields": [{"name": "apikey", "value": ""}]})
        self.assertEqual({r["name"] for r in b.provision(api, "http://prowlarr:9696", "k")}, {"anidex", "thepiratebay"})
        self.assertEqual(api.creates, 2)
        result = b.provision(api, "http://prowlarr:9696", "k", None, None, {"c411": "tracker-secret"})
        self.assertEqual({r["name"] for r in result}, {"anidex", "thepiratebay", "c411"})
        created = next(i for i in api.existing if i.get("definitionName") == "c411")
        self.assertEqual(created["fields"][0]["value"], "tracker-secret")
        self.assertNotIn("tracker-secret", json.dumps(result))
        self.assertEqual([r["name"] for r in result if r.get("indexerOnly")], ["c411"])
        self.assertEqual(b.provision(api, "http://prowlarr:9696", "k", None, None, {"c411": "other"}), result)
        self.assertEqual(api.creates, 3)

    def test_api_failure_does_not_leak_key(self):
        api = b.API("http://gateway", "sensitive-key")
        with patch.object(b.urllib.request, "urlopen", side_effect=OSError("sensitive-key")):
            with self.assertRaises(b.APIError) as error: api("indexer")
        self.assertNotIn("sensitive-key", str(error.exception))

    def test_manifest_is_private_and_atomic(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "indexers.json"
            b.write_manifest(path, [{"apiKey": "secret"}])
            self.assertEqual(json.loads(path.read_text()), [{"apiKey": "secret"}])
            self.assertEqual(path.stat().st_mode & 0o777, 0o400)
            self.assertFalse(path.with_suffix(".tmp").exists())


if __name__ == "__main__": unittest.main()
