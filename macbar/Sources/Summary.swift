import Foundation

// Mirror of the panel's GET /api/summary payload.
struct Summary: Codable, Equatable {
    struct Pool: Codable, Equatable {
        var mount: String
        var used: UInt64
        var total: UInt64
    }
    struct MCServer: Codable, Equatable {
        var id: String
        var state: String
        var players: Int
        var maxPlayers: Int
    }

    var hostname: String
    var mock: Bool
    var now: Int64
    var bootTime: Int64
    var cpu: Double
    var memUsed: UInt64
    var memTotal: UInt64
    var pool: Pool?
    var disksFlagged: Int
    var disksUnhealthy: Int
    var servicesFailed: [String]
    var mc: [MCServer]
    var plexConfigured: Bool
    var plexStreams: Int
    // Media apps rollup (absent on older panel versions).
    var appsConfigured: Bool?
    var appsQueue: Int?
    var appsIssues: Int?
    var requestsPending: Int?
    var jobRunning: String?
}

enum Format {
    static func bytes(_ v: UInt64) -> String {
        let units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"]
        var value = Double(v)
        var i = 0
        while value >= 1024 && i < units.count - 1 {
            value /= 1024
            i += 1
        }
        let digits = value >= 100 || i == 0 ? 0 : 1
        return String(format: "%.\(digits)f %@", value, units[i])
    }

    static func uptime(sinceMs bootTime: Int64) -> String {
        guard bootTime > 0 else { return "—" }
        let secs = Int(Date().timeIntervalSince1970) - Int(bootTime / 1000)
        if secs < 0 { return "—" }
        let d = secs / 86400, h = (secs % 86400) / 3600, m = (secs % 3600) / 60
        if d > 0 { return "\(d)d \(h)h" }
        if h > 0 { return "\(h)h \(m)m" }
        return "\(m)m"
    }
}
