"""Every user-facing string of the macOS app goes through L(key, English) and has an entry
in the English and the Spanish Localizable.strings (the project's i18n rule)."""
import glob
import os
import re
import unittest

ROOT = os.path.join(os.path.dirname(__file__), "..", "macos")
CALL = re.compile(r'\bL\("([^"]+)",\s*"((?:[^"\\]|\\.)*)"')
ENTRY = re.compile(r'^"([^"]+)"\s*=\s*"((?:[^"\\]|\\.)*)";$')


def strings(lang):
    with open(os.path.join(ROOT, "Resources", lang + ".lproj", "Localizable.strings"), encoding="utf-8") as f:
        return dict(m.groups() for m in map(ENTRY.match, f.read().splitlines()) if m)


class MacStrings(unittest.TestCase):
    def test_every_key_is_translated(self):
        used = {}
        for path in glob.glob(os.path.join(ROOT, "Sources", "Refract", "*.swift")):
            with open(path, encoding="utf-8") as f:
                used.update(CALL.findall(f.read()))
        self.assertTrue(used)
        en, es = strings("en"), strings("es")
        self.assertEqual(set(used), set(en), "en.lproj out of date")
        self.assertEqual(set(used), set(es), "es.lproj out of date")
        for key, value in used.items():
            self.assertEqual(en[key], value, key)
            self.assertNotEqual(es[key], "", key)
            self.assertEqual(en[key].count("%"), es[key].count("%"), "format specifiers of " + key)


if __name__ == "__main__":
    unittest.main()
