"""Idempotent provisioning. Never log API bodies, URLs or credentials."""
import copy
import json
import os
from pathlib import Path
import secrets
import sys
import time
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET

UID = 10001
TARGETS = (("anidex", "Anidex"), ("thepiratebay", "Cardigann"))


def initialize(directory):
    directory = Path(directory)
    directory.mkdir(parents=True, exist_ok=True)
    path = directory / "config.xml"
    if not path.exists():
        root = ET.Element("Config")
        for name, value in {"ApiKey": secrets.token_hex(16), "AuthenticationMethod": "External",
                            "BindAddress": "*", "Port": "9696", "LaunchBrowser": "False",
                            "AnalyticsEnabled": "False", "LogLevel": "info"}.items():
            ET.SubElement(root, name).text = value
        with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as out:
            ET.ElementTree(root).write(out, encoding="utf-8", xml_declaration=True)
        os.chmod(path, 0o600)
    if os.geteuid() == 0:
        os.chown(directory, UID, UID)
        os.chown(path, UID, UID)
    return path


class APIError(Exception):
    pass


class API:
    def __init__(self, base, key):
        self.base, self.key = base.rstrip("/"), key

    def __call__(self, path, payload=None, method=None):
        request = urllib.request.Request(self.base + "/api/v1/" + path,
            data=None if payload is None else json.dumps(payload).encode(),
            headers={"X-Api-Key": self.key, "Content-Type": "application/json"}, method=method)
        try:
            with urllib.request.urlopen(request, timeout=15) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            raise APIError("Prowlarr API HTTP " + str(error.code)) from None
        except (OSError, ValueError):
            raise APIError("Prowlarr API unavailable or invalid response") from None


def matches(resource, name, implementation):
    return resource.get("implementation", "").lower() == implementation.lower() and (
        implementation != "Cardigann" or resource.get("definitionName", "").lower() == name)


def provision(api, base, key, pending_path=None):
    schemas = api("indexer/schema")
    existing = api("indexer")
    profiles = api("appprofile")
    pending = set(json.loads(Path(pending_path).read_text())) if pending_path and Path(pending_path).exists() else set()
    def save_pending():
        if pending_path:
            write_manifest(pending_path, sorted(pending))
    result = []
    for name, implementation in TARGETS:
        found = next((r for r in existing if matches(r, name, implementation)), None)
        if found is None:
            schema = next((s for s in schemas if matches(s, name, implementation)), None)
            if schema is None:
                print("Prowlarr definition absent: " + name, flush=True)
                continue
            payload = copy.deepcopy(schema)
            payload.pop("id", None)
            payload.update(name="Gazes " + name, enable=False)
            for field in payload.get("fields", []):
                if field["name"] == "baseUrl" and not field.get("value"):
                    field["value"] = schema["indexerUrls"][0]
                if field["name"] == "torrentBaseSettings.preferMagnetUrl":
                    field["value"] = True
            if profiles:
                payload["appProfileId"] = profiles[0]["id"]
            found = api("indexer?forceSave=true", payload)
            existing.append(found)
            pending.add(found["id"])
            save_pending()
        if found["id"] in pending:
            updated = copy.deepcopy(found)
            updated["enable"] = True
            found = api("indexer/" + str(found["id"]) + "?forceSave=true", updated, method="PUT")
            pending.remove(found["id"])
            save_pending()
        # Reuse existing providers without changing any manually configured settings.
        if found.get("enable", False):
            result.append({"name": name, "endpoint": base.rstrip("/") + "/" + str(found["id"]) + "/api", "apiKey": key})
    return result


def write_manifest(path, providers):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(".tmp")
    descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, "w") as out:
        json.dump(providers, out)
    if os.geteuid() == 0:
        os.chown(temporary, UID, UID)
    os.chmod(temporary, 0o400)
    os.replace(temporary, path)


def main():
    directory = os.getenv("PROWLARR_CONFIG", "/config")
    if sys.argv[1] == "init":
        initialize(directory)
        return
    key = ET.parse(Path(directory) / "config.xml").getroot().findtext("ApiKey")
    if not key:
        raise APIError("Prowlarr API key missing from persistent configuration")
    base = os.getenv("PROWLARR_URL", "http://prowlarr:9696")
    api = API(base, key)
    for attempt in range(60):
        try:
            api("system/status")
            break
        except APIError:
            if attempt == 59:
                raise APIError("Prowlarr did not become ready") from None
            time.sleep(2)
    providers = provision(api, base, key, "/shared/pending.json")
    write_manifest(os.getenv("INDEXER_CONFIG_FILE", "/shared/indexers.json"), providers)
    print("Prowlarr configured: " + str(len(providers)) + " providers", flush=True)


if __name__ == "__main__":
    try:
        main()
    except (APIError, OSError, ET.ParseError):
        print("Prowlarr bootstrap failed; check service health and persistent configuration", file=sys.stderr)
        sys.exit(1)
