import Foundation

/// Every user-facing string goes through L(key, English), as in the app target. Each key needs
/// an entry in Resources/en.lproj and Resources/es.lproj (scripts/test_macos_strings.py checks it).
func L(_ key: String, _ value: String) -> String {
    NSLocalizedString(key, value: value, comment: "")
}
