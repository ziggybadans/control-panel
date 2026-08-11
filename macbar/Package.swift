// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "PanelBar",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(name: "PanelBar", path: "Sources")
    ]
)
