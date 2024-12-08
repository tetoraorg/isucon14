#!/usr/bin/env python3

from pathlib import Path
import json
import re
import subprocess
import yaml


def extract_git_repository_name():
    try:
        remote_url = subprocess.check_output(
            ["git", "config", "--get", "remote.origin.url"],
            text=True,
        ).strip()
        match = re.search(r"[:/]([^/]+)/([^/]+?)(?:\.git)?$", remote_url)
        if match:
            owner, repo = match.groups()
            return f"{owner}/{repo}"
        raise Exception("Failed to extract repository name")
    except subprocess.CalledProcessError:
        raise Exception("Failed to get git remote URL")


if __name__ == "__main__":
    host_dirs = Path(__file__).parent.parent.glob("s[0-9]*/")
    hosts = []
    hostvars = {}
    for host_dir in sorted(host_dirs):
        host = host_dir.stem
        hosts.append(host)
        with open(host_dir / "host_vars.yml") as f:
            vars = yaml.safe_load(f)
            ssh_config = (
                f"Host {host}\n"
                f"    HostName {vars['ansible_host']}\n"
                f"    User {vars['ansible_user']}\n"
            )
            hostvars[host] = vars
            with open(Path(__file__).parent / ".ssh_config", "a") as ssh_config_file:
                ssh_config_file.write(ssh_config)

    inventory = {
        "all": {
            "hosts": hosts,
            "vars": {
                "repository": extract_git_repository_name(),
            },
        },
        "_meta": {
            "hostvars": hostvars,
        },
    }

    print(json.dumps(inventory, indent=2))
