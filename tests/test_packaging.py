"""Check package metadata and generated Brew formula inputs."""
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class PackagingTests(unittest.TestCase):
    def test_deb_metadata_and_payload_are_passed_to_dpkg(self):
        with tempfile.TemporaryDirectory(prefix="narrate package ") as temp:
            root = Path(temp)
            fakebin = root / "bin"
            fakebin.mkdir()
            capture = root / "control"
            (fakebin / "dpkg-deb").write_text(
                '#!/bin/sh\nset -eu\n'
                'cp "$3/DEBIAN/control" "$CAPTURE_CONTROL"\n'
                'test -x "$3/usr/bin/narrate"\n'
                ': > "$4"\n'
            )
            (fakebin / "dpkg-deb").chmod(0o700)
            binary = root / "narrate"
            binary.write_text("#!/bin/sh\nexit 0\n")
            binary.chmod(0o700)
            env = dict(os.environ, PATH=str(fakebin) + os.pathsep + os.environ["PATH"],
                       CAPTURE_CONTROL=str(capture))
            output = root / "dist/narrate_linux_amd64.deb"
            result = subprocess.run(
                ["sh", str(ROOT / "scripts/package-deb.sh"), "1.0.0", "amd64", str(binary), str(output)],
                env=env, text=True, capture_output=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            control = capture.read_text()
            self.assertIn("Package: narrate\n", control)
            self.assertIn("Version: 1.0.0\n", control)
            self.assertIn("Architecture: amd64\n", control)
            self.assertTrue(output.exists())

    def test_formula_uses_release_checksums_for_all_targets(self):
        with tempfile.TemporaryDirectory(prefix="narrate formula ") as temp:
            root = Path(temp)
            dist = root / "dist"
            dist.mkdir()
            for target in ("darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"):
                binary = dist / f"narrate_1.0.0_{target}"
                data = f"binary for {target}".encode()
                binary.write_bytes(data)
                (dist / f"narrate_1.0.0_{target}.sha256").write_text(hashlib.sha256(data).hexdigest() + "\n")
            formula = root / "Formula/narrate.rb"
            result = subprocess.run(
                ["sh", str(ROOT / "scripts/update-homebrew-formula.sh"), "1.0.0", str(dist), str(formula)],
                text=True, capture_output=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            text = formula.read_text()
            for target in ("darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"):
                self.assertIn(f"narrate_#{{version}}_{target}", text)
            self.assertEqual(text.count('sha256 "'), 4)


if __name__ == "__main__":
    unittest.main()
