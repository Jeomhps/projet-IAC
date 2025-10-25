import requests
import sys
import os
import csv
from typing import List, Dict, Any

try:
    import yaml  # PyYAML
except Exception:
    yaml = None

import urllib3
urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

API_URL = os.getenv("API_URL", "https://localhost/machines")

def _parse_txt(path: str) -> List[Dict[str, Any]]:
    items = []
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            parts = line.split()
            if len(parts) != 5:
                print(f"Skipping invalid line: {line}")
                continue
            name, host, port, user, password = parts
            items.append({"name": name, "host": host, "port": int(port), "user": user, "password": password})
    return items

def _parse_csv(path: str) -> List[Dict[str, Any]]:
    items = []
    with open(path, newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        header = [h.lower() for h in reader.fieldnames or []]
        required = {"name", "host", "port", "user", "password"}
        # If no header or missing fields, fall back to positional columns
        if not header or not required.issubset(set(header)):
            f.seek(0)
            reader2 = csv.reader(f)
            for row in reader2:
                if not row or row[0].strip().startswith("#"):
                    continue
                if len(row) < 5:
                    print(f"Skipping invalid CSV row: {row}")
                    continue
                name, host, port, user, password = [c.strip() for c in row[:5]]
                items.append({"name": name, "host": host, "port": int(port), "user": user, "password": password})
            return items

        for row in reader:
            name = row.get("name", "").strip()
            host = row.get("host", "").strip()
            port = row.get("port", "").strip()
            user = row.get("user", "").strip()
            password = row.get("password", "").strip()
            if not all([name, host, port, user, password]):
                print(f"Skipping incomplete row: {row}")
                continue
            items.append({"name": name, "host": host, "port": int(port), "user": user, "password": password})
    return items

def _parse_yaml(path: str) -> List[Dict[str, Any]]:
    if yaml is None:
        raise RuntimeError("PyYAML is not installed. Please add 'PyYAML' to requirements.txt or use .csv/.txt.")
    with open(path, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f)
    if data is None:
        return []
    if isinstance(data, dict) and "machines" in data:
        data = data["machines"]
    if not isinstance(data, list):
        raise ValueError("YAML file must be a list of machines or contain a top-level 'machines' list.")
    items = []
    for m in data:
        name = str(m.get("name", "")).strip()
        host = str(m.get("host", "")).strip()
        port = m.get("port", "")
        user = str(m.get("user", "")).strip()
        password = str(m.get("password", "")).strip()
        if not all([name, host, port, user, password]):
            print(f"Skipping incomplete machine entry: {m}")
            continue
        items.append({"name": name, "host": host, "port": int(port), "user": user, "password": password})
    return items

def parse_machines_file(path: str) -> List[Dict[str, Any]]:
    ext = os.path.splitext(path)[1].lower()
    if ext in (".yml", ".yaml"):
        return _parse_yaml(path)
    if ext == ".csv":
        return _parse_csv(path)
    return _parse_txt(path)

def main(machines_file: str) -> None:
    machines_path = os.path.abspath(machines_file)
    if not os.path.exists(machines_path):
        print(f"Error: {machines_path} does not exist.")
        sys.exit(1)

    try:
        machines = parse_machines_file(machines_path)
    except Exception as e:
        print(f"Failed to parse {machines_path}: {e}")
        sys.exit(1)

    if not machines:
        print("No machines to register.")
        return

    for m in machines:
        try:
            resp = requests.post(API_URL, json=m, verify=False)
            if resp.status_code == 201:
                print(f"Added {m['name']}")
            else:
                print(f"Failed to add {m['name']}: {resp.text}")
        except requests.RequestException as e:
            print(f"Failed to add {m['name']}: {e}")

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python utils/register_machines.py <machines_file>")
        sys.exit(1)
    main(sys.argv[1])
