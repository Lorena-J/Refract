import Foundation

/// Where Refract.app keeps the server and its tools. The server finds its tools by
/// itself, relative to its own executable (`<exe dir>/../libexec/refract`, #53), so
/// this layout is a contract with internal/tools. It is under Resources because codesign
/// treats every file under Helpers/ as nested code, and Git's tree has scripts and data:
///
///     Contents/MacOS/Refract                      the app
///     Contents/Resources/Server/bin/refract                the server
///     Contents/Resources/Server/libexec/refract/...        tectonic synctex rg git/...
public struct BundleLayout {
    public let contents: URL

    public init(contents: URL) { self.contents = contents }

    public init(appBundle: Bundle = .main) {
        self.contents = appBundle.bundleURL.appendingPathComponent("Contents", isDirectory: true)
    }

    public var server: URL { contents.appendingPathComponent("Resources/Server/bin/refract") }
    public var toolsDirectory: URL { contents.appendingPathComponent("Resources/Server/libexec/refract", isDirectory: true) }

    /// Programs the app can't work without, relative to `toolsDirectory`.
    public static let requiredTools = ["tectonic", "synctex", "rg", "git/bin/git"]

    /// Names of what is missing or not executable: empty when the install is whole.
    public func missing() -> [String] {
        let fm = FileManager.default
        var names: [String] = []
        if !fm.isExecutableFile(atPath: server.path) { names.append("refract") }
        for tool in Self.requiredTools
        where !fm.isExecutableFile(atPath: toolsDirectory.appendingPathComponent(tool).path) {
            names.append((tool as NSString).lastPathComponent)
        }
        var isDir: ObjCBool = false
        let helpers = toolsDirectory.appendingPathComponent("git/libexec/git-core").path
        if !(fm.fileExists(atPath: helpers, isDirectory: &isDir) && isDir.boolValue) { names.append("git-core") }
        return names
    }
}
