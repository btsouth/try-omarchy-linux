from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("smoke-guest.py")
SPEC = importlib.util.spec_from_file_location("smoke_guest", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
smoke_guest = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(smoke_guest)


class ParseFactsTests(unittest.TestCase):
    def test_ignores_echoed_placeholders_and_keeps_last_real_value(self) -> None:
        transcript = (
            b"printf 'TRYOMARCHY_FACT:yay:%s\\n' \"$(pacman -Q yay)\"\r\n"
            b"\x1b[?2004lTRYOMARCHY_FACT:yay:missing\r\n"
            b"TRYOMARCHY_FACT:sshd:inactive\r\n"
            b"TRYOMARCHY_FACT:yay:present\r\n"
        )
        self.assertEqual(smoke_guest.parse_facts(transcript), {"yay": "present", "sshd": "inactive"})

    def test_tolerates_bad_utf8_and_rejects_quoted_or_escaped_values(self) -> None:
        transcript = (
            b"\xffTRYOMARCHY_FACT:foreign:0\n"
            b"TRYOMARCHY_FACT:quoted:'present'\n"
            b"TRYOMARCHY_FACT:escaped:present\\later\n"
        )
        self.assertEqual(smoke_guest.parse_facts(transcript), {"foreign": "0"})


class GuestRevisionTests(unittest.TestCase):
    def test_unified_revision_is_read_from_appended_patches(self):
        self.assertEqual(smoke_guest.guest_compat_revision(), 61)

    def test_linux_payload_checks_follow_delivery_revisions(self):
        self.assertEqual(smoke_guest.linux_guest_payload_checks(49), {})
        previous = set()
        for revision, name in [(50, "vulkan"), (51, "idle"), (52, "display"),
                               (53, "ready"), (54, "app"), (55, "notices"),
                               (56, "labels-hook"), (57, "nightlight"), (58, "notifications"),
                               (59, "health"), (60, "live-update"), (61, "nightlight-adoption")]:
            with self.subTest(revision=revision):
                checks = smoke_guest.linux_guest_payload_checks(revision)
                self.assertEqual(set(checks) - previous, {f"linux-{name}-payload"})
                self.assertTrue(all(command.endswith(" && echo yes || echo no") for command in checks.values()))
                previous = set(checks)


if __name__ == "__main__":
    unittest.main()
