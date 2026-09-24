"""Exercise the real launcher with a fake HTTPS transport; never fetch or play audio."""
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class BootstrapTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="narrate bootstrap ")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        version = (ROOT / "scripts/cli-version").read_text().strip()
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
                        CLAUDE_PLUGIN_DATA=str(self.root / "data"),
                        FIXTURE_ROOT=str(self.root), TEST_RELEASE_TAG=f"v{version}")
        self.payload = b'#!/bin/sh\nprintf "cwd=%s\\n" "$PWD"\nprintf "arg=%s\\n" "$@"\ncat\nexit 7\n'
        (self.root / "payload").write_bytes(self.payload)
        (self.root / "checksum").write_text(hashlib.sha256(self.payload).hexdigest() + "\n")
        self.script("uname", '#!/bin/sh\ncase "$1" in -s) echo "${TEST_OS:-Darwin}";; -m) echo "${TEST_ARCH:-arm64}";; esac\n')
        self.script("curl", '''#!/bin/sh
set -eu
echo call >> "$FIXTURE_ROOT/calls"
output=
write_out=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) output=$2; shift 2;;
    --write-out) write_out=$2; shift 2;;
    http*) url=$1; shift;;
    *) shift;;
  esac
done
echo "$url" >> "$FIXTURE_ROOT/urls"
if [ "$url" = https://github.com/narrate-it/narrate/releases/latest ]; then
  if [ "${FAIL_DOWNLOAD:-0}" != 0 ]; then exit 22; fi
  printf 'https://github.com/narrate-it/narrate/releases/tag/%s' "${TEST_RELEASE_TAG:-v0.1.0}"
else
  [ "${FAIL_DOWNLOAD:-0}" = 0 ] || exit 22
  case "$url" in
    *.sha256) cp "$FIXTURE_ROOT/checksum" "$output";;
    *) cp "$FIXTURE_ROOT/payload" "$output";;
  esac
fi
''')

    def script(self, name, content):
        path = self.bin / name
        path.write_text(content)
        path.chmod(0o700)

    def run_cli(self, *args):
        return subprocess.run(["sh", str(ROOT / "scripts/narrate.sh"), *args],
                              cwd=self.root, env=self.env, input="spoken input\n",
                              text=True, capture_output=True)

    def test_first_use_and_offline_reuse_preserve_invocation(self):
        for offline in (False, True):
            self.env["FAIL_DOWNLOAD"] = str(int(offline))
            result = self.run_cli("--verbatim", "a path with spaces", "$(echo unsafe)")
            self.assertEqual(result.returncode, 7, result.stderr)
            self.assertIn("cwd=" + str(self.root.resolve()), result.stdout)
            self.assertIn("arg=a path with spaces\narg=$(echo unsafe)\nspoken input", result.stdout)
        self.assertEqual((self.root / "calls").read_text().splitlines(), ["call", "call", "call"])
        self.assertFalse(list((self.root / "data").rglob(".download.*")))

    def test_bad_checksum_never_installs_or_executes(self):
        (self.root / "checksum").write_text("0" * 64)
        result = self.run_cli()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Checksum mismatch", result.stderr)
        self.assertEqual(result.stdout, "")
        self.assertFalse(list((self.root / "data").rglob("narrate")))

    def test_download_failure_can_retry(self):
        self.env["FAIL_DOWNLOAD"] = "1"
        self.assertIn("Download failed", self.run_cli().stderr)
        self.assertFalse(list((self.root / "data").rglob(".download.*")))
        self.env["FAIL_DOWNLOAD"] = "0"
        self.assertEqual(self.run_cli().returncode, 7)

    def test_malformed_checksum_rejected(self):
        (self.root / "checksum").write_text("not a digest")
        self.assertIn("Invalid release checksum", self.run_cli().stderr)

    def test_all_platform_assets(self):
        version = (ROOT / "scripts/cli-version").read_text().strip()
        for os_name, arch, asset_os, asset_arch in [
            ("Darwin", "arm64", "darwin", "arm64"),
            ("Darwin", "x86_64", "darwin", "amd64"),
            ("Linux", "aarch64", "linux", "arm64"),
            ("Linux", "x86_64", "linux", "amd64"),
        ]:
            with self.subTest(os=os_name, arch=arch):
                self.env.update(TEST_OS=os_name, TEST_ARCH=arch)
                result = self.run_cli()
                self.assertEqual(result.returncode, 7)
                expected = f"https://github.com/narrate-it/narrate/releases/download/v{version}/narrate_{version}_{asset_os}_{asset_arch}"
                self.assertIn(expected, (self.root / "urls").read_text().splitlines())

    def test_checks_for_new_releases_after_six_hours(self):
        self.assertEqual(self.run_cli().returncode, 7)
        self.env["TEST_NOW"] = "2000000000"
        self.env["TEST_RELEASE_TAG"] = "v0.2.0"
        self.script("date", '#!/bin/sh\necho "${TEST_NOW:-1000000000}"\n')
        result = self.run_cli()
        self.assertEqual(result.returncode, 7)
        self.assertIn("releases/download/v0.2.0/narrate_0.2.0_darwin_arm64", (self.root / "urls").read_text())

    def test_current_installed_binary_is_used_if_no_release_is_available(self):
        fallback = self.root / "installed-narrate"
        version = (ROOT / "scripts/cli-version").read_text().strip()
        fallback.write_text(f'#!/bin/sh\nif [ "$1" = --version ]; then echo "narrate version {version}"; exit; fi\necho fallback "$@"\n')
        fallback.chmod(0o700)
        self.env["NARRATE_FALLBACK_BIN"] = str(fallback)
        self.env["FAIL_DOWNLOAD"] = "1"
        result = self.run_cli("--verbatim", "hello")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("fallback --verbatim hello", result.stdout)
        self.assertEqual((self.root / "urls").read_text().splitlines(), [
            "https://github.com/narrate-it/narrate/releases/latest",
        ])

    def test_package_managed_binary_at_current_release_is_not_replaced(self):
        fallback = self.root / "installed-narrate"
        fallback.write_text('#!/bin/sh\nif [ "$1" = --version ]; then echo "narrate version 0.2.0"; exit; fi\necho managed "$@"\n')
        fallback.chmod(0o700)
        self.env["NARRATE_FALLBACK_BIN"] = str(fallback)
        self.env["TEST_RELEASE_TAG"] = "v0.2.0"
        result = self.run_cli("--verbatim", "hello")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("managed --verbatim hello", result.stdout)
        self.assertEqual((self.root / "urls").read_text().splitlines(), [
            "https://github.com/narrate-it/narrate/releases/latest",
        ])

    def test_unsupported_platform_does_not_download(self):
        self.env["TEST_OS"] = "Windows_NT"
        self.assertIn("macOS and Linux only", self.run_cli().stderr)
        self.assertFalse((self.root / "calls").exists())

    def test_symlinked_launcher_resolves_its_data_files(self):
        linked = self.bin / "narrate"
        linked.symlink_to(ROOT / "scripts/narrate.sh")
        result = subprocess.run(["sh", str(linked), "--version"], cwd=self.root, env=self.env,
                                input="", text=True, capture_output=True)
        self.assertEqual(result.returncode, 7, result.stderr)
        self.assertIn("https://github.com/narrate-it/narrate/releases/latest", (self.root / "urls").read_text())

    def test_fallback_data_directory(self):
        del self.env["CLAUDE_PLUGIN_DATA"]
        self.env["XDG_DATA_HOME"] = str(self.root / "xdg")
        self.assertEqual(self.run_cli().returncode, 7)
        self.assertEqual(len(list((self.root / "xdg/narrate-plugin").rglob("narrate"))), 1)


if __name__ == "__main__":
    unittest.main()
