"""Exercise launcher failure/ownership boundaries without a container daemon."""

import os
from pathlib import Path
import subprocess
import socket
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class IntegrationLauncherTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="pletka-launcher-test-")
        self.addCleanup(self.tmp.cleanup)
        self.directory = Path(self.tmp.name)
        self.log = self.directory / "calls"
        self.env = dict(os.environ)
        for key in ("TEST_DATABASE_URL", "DOCKER_CONTEXT", "TEST_POSTGRES_IMAGE"):
            self.env.pop(key, None)
        self.env.update(
            PATH=f"{self.directory}:{os.environ['PATH']}",
            DOCKER_HOST="unix:///test/podman.sock",
            TEST_CONTAINER_RUNTIME="podman",
            TEST_CALLS=str(self.log),
            REQUIRE_DB="0",
        )
        self.script("podman", '''
printf 'runtime %s\\n' "$*" >> "$TEST_CALLS"
if [[ "$*" == *" info" ]]; then exit "${FAKE_RUNTIME_STATUS:-0}"; fi
if [[ "$*" == *" rm "* ]]; then exit "${FAKE_CLEANUP_STATUS:-0}"; fi
''')
        self.script("go", '''
printf 'go db=%s required=%s name=%s image=%s args=%s\\n' \\
    "${TEST_DATABASE_URL:-none}" "$REQUIRE_DB" "${PLETKA_TEST_CONTAINER_NAME:-none}" \\
    "${TEST_POSTGRES_IMAGE:-none}" "$*" >> "$TEST_CALLS"
exit "${FAKE_TEST_STATUS:-0}"
''')

    def script(self, name, body):
        path = self.directory / name
        path.write_text("#!/usr/bin/env bash\nset -eu\n" + body)
        path.chmod(0o755)

    def run_launcher(self, **overrides):
        return subprocess.run(
            ["bash", str(ROOT / "test/integration.sh"), "./pkg/weave/release"],
            env=self.env | overrides,
            capture_output=True,
            text=True,
            check=False,
        )

    def calls(self):
        return self.log.read_text() if self.log.exists() else ""

    def test_failed_tests_preserve_status_and_clean_only_their_container(self):
        result = self.run_launcher(FAKE_TEST_STATUS="7")
        self.assertEqual(result.returncode, 7, result.stderr)
        calls = self.calls()
        self.assertIn("required=1", calls)
        self.assertIn("-tags=integration -count=1 ./pkg/weave/release", calls)
        go_line = next(line for line in calls.splitlines() if line.startswith("go "))
        name = go_line.split("name=", 1)[1].split()[0]
        self.assertTrue(name.startswith("pletka-testdb-pletka-test."))
        self.assertIn(f"--remote --url unix:///test/podman.sock rm -f -v {name}", calls)
        self.assertNotIn("pletka-testdb-pg18", calls)
        self.assertIn((ROOT / "internal/testdb/postgres-image.txt").read_text().strip(), calls)

    def test_missing_runtime_fails_before_tests(self):
        result = self.run_launcher(FAKE_RUNTIME_STATUS="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("go db=", self.calls())
        self.assertNotIn(" rm ", self.calls())

    def test_explicit_database_never_contacts_or_cleans_container_runtime(self):
        result = self.run_launcher(TEST_DATABASE_URL="postgres://test-only/server")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("required=1", self.calls())
        self.assertNotIn("runtime", self.calls())

    def test_each_run_gets_its_own_container(self):
        first = self.run_launcher()
        second = self.run_launcher()
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(second.returncode, 0, second.stderr)
        names = [line.split("name=", 1)[1].split()[0]
                 for line in self.calls().splitlines() if line.startswith("go ")]
        self.assertEqual(len(set(names)), 2)

    def test_cleanup_failure_fails_otherwise_successful_run(self):
        result = self.run_launcher(FAKE_CLEANUP_STATUS="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Could not remove test container", result.stderr)

    def test_docker_uses_explicit_endpoint_for_tests_and_cleanup(self):
        self.script("docker", '''
printf 'docker endpoint=%s args=%s\\n' "$DOCKER_HOST" "$*" >> "$TEST_CALLS"
''')
        result = self.run_launcher(TEST_CONTAINER_RUNTIME="docker", DOCKER_HOST="unix:///test/docker.sock")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("docker endpoint=unix:///test/docker.sock args=--host unix:///test/docker.sock rm -f -v", self.calls())

    def test_podman_docker_shim_detected_even_with_docker_version_banner(self):
        sock = socket.socket(socket.AF_UNIX)
        self.addCleanup(sock.close)
        socket_path = self.directory / "podman.sock"
        sock.bind(str(socket_path))
        self.script("docker", '''
if [[ "$1" == info ]]; then printf '%s\\n' "$FAKE_PODMAN_SOCKET"; fi
if [[ "$1" == --version ]]; then echo 'docker version 5.8.7'; fi
''')
        result = self.run_launcher(TEST_CONTAINER_RUNTIME="", DOCKER_HOST="", FAKE_PODMAN_SOCKET=str(socket_path))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f"--remote --url unix://{socket_path} rm -f -v", self.calls())


if __name__ == "__main__":
    unittest.main()
