import SwiftUI

// The glanceable popover shown when clicking the menu bar item.
struct PanelView: View {
    @EnvironmentObject var model: PanelModel

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if model.showSettings {
                SettingsView()
            } else {
                content
            }
        }
        .frame(width: 296)
    }

    @ViewBuilder private var content: some View {
        header
        Divider()
        switch model.status {
        case .unconfigured:
            message("Point me at your control panel", detail: "Open settings to set the URL.")
        case .authNeeded:
            message("Authentication needed", detail: "Set the panel password in settings.")
        case .unreachable(let err):
            message("Can't reach the panel", detail: err)
            if model.summary != nil {
                Divider()
                stats
            }
        case .ok, .degraded:
            stats
        }
        Divider()
        footer
    }

    private var header: some View {
        HStack(spacing: 8) {
            StatusDot(status: model.status)
            Text(model.summary?.hostname ?? "control panel")
                .font(.system(size: 13, weight: .semibold))
            if model.summary?.mock == true {
                Text("mock")
                    .font(.system(size: 9, weight: .medium))
                    .padding(.horizontal, 5).padding(.vertical, 1)
                    .background(Capsule().fill(.quaternary))
            }
            Spacer()
            if let s = model.summary {
                Text("up \(Format.uptime(sinceMs: s.bootTime))")
                    .font(.system(size: 11)).foregroundStyle(.secondary)
            }
        }
        .padding(.horizontal, 12).padding(.vertical, 9)
    }

    @ViewBuilder private var stats: some View {
        if let s = model.summary {
            VStack(alignment: .leading, spacing: 7) {
                MeterRow(
                    label: "CPU",
                    value: s.cpu / 100,
                    detail: String(format: "%.0f%%", s.cpu)
                )
                MeterRow(
                    label: "Memory",
                    value: s.memTotal > 0 ? Double(s.memUsed) / Double(s.memTotal) : 0,
                    detail: "\(Format.bytes(s.memUsed)) / \(Format.bytes(s.memTotal))"
                )
                if let pool = s.pool {
                    MeterRow(
                        label: "Pool",
                        value: pool.total > 0 ? Double(pool.used) / Double(pool.total) : 0,
                        detail: "\(Format.bytes(pool.used)) / \(Format.bytes(pool.total))"
                    )
                }

                if case .degraded(let why) = model.status {
                    Label(why, systemImage: "exclamationmark.triangle.fill")
                        .font(.system(size: 11, weight: .medium))
                        .foregroundStyle(.orange)
                } else if s.disksFlagged > 0 {
                    Label("\(s.disksFlagged) disk(s) with SMART warnings", systemImage: "exclamationmark.triangle")
                        .font(.system(size: 11))
                        .foregroundStyle(.orange)
                }

                if !s.mc.isEmpty {
                    sectionLabel("MINECRAFT")
                    ForEach(s.mc, id: \.id) { srv in
                        HStack(spacing: 6) {
                            Circle()
                                .fill(mcColor(srv.state))
                                .frame(width: 6, height: 6)
                            Text(srv.id).font(.system(size: 12))
                            Spacer()
                            Text(srv.state == "running"
                                 ? "\(srv.players)/\(srv.maxPlayers) online"
                                 : srv.state)
                                .font(.system(size: 11).monospacedDigit())
                                .foregroundStyle(.secondary)
                        }
                    }
                }

                if s.plexConfigured {
                    sectionLabel("PLEX")
                    HStack(spacing: 6) {
                        Image(systemName: "play.tv").font(.system(size: 11))
                            .foregroundStyle(.secondary)
                        Text(s.plexStreams == 0
                             ? "idle"
                             : "\(s.plexStreams) active stream\(s.plexStreams == 1 ? "" : "s")")
                            .font(.system(size: 12))
                        Spacer()
                    }
                }

                if s.appsConfigured == true {
                    sectionLabel("MEDIA APPS")
                    HStack(spacing: 10) {
                        Image(systemName: "arrow.down.circle").font(.system(size: 11))
                            .foregroundStyle(.secondary)
                        Text(appsLine(s)).font(.system(size: 12))
                        Spacer()
                        if let issues = s.appsIssues, issues > 0 {
                            Label("\(issues)", systemImage: "exclamationmark.triangle.fill")
                                .font(.system(size: 11))
                                .foregroundStyle(.orange)
                        }
                    }
                }

                if let job = s.jobRunning {
                    Label(job, systemImage: "gearshape.arrow.triangle.2.circlepath")
                        .font(.system(size: 11))
                        .foregroundStyle(.secondary)
                }
            }
            .padding(.horizontal, 12).padding(.vertical, 9)
        }
    }

    private func sectionLabel(_ text: String) -> some View {
        Text(text)
            .font(.system(size: 9.5, weight: .semibold))
            .kerning(0.6)
            .foregroundStyle(.tertiary)
            .padding(.top, 3)
    }

    private func message(_ title: String, detail: String) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(title).font(.system(size: 12, weight: .medium))
            Text(detail).font(.system(size: 11)).foregroundStyle(.secondary)
        }
        .padding(.horizontal, 12).padding(.vertical, 10)
    }

    private var footer: some View {
        HStack {
            Button {
                model.openPanel()
            } label: {
                Label("Open Panel", systemImage: "rectangle.on.rectangle")
            }
            Spacer()
            Button {
                model.showSettings = true
            } label: {
                Image(systemName: "gearshape")
            }
            .help("Settings")
            Button {
                NSApplication.shared.terminate(nil)
            } label: {
                Image(systemName: "power")
            }
            .help("Quit")
        }
        .buttonStyle(.borderless)
        .controlSize(.small)
        .font(.system(size: 12))
        .padding(.horizontal, 12).padding(.vertical, 8)
    }

    private func appsLine(_ s: Summary) -> String {
        var parts: [String] = []
        if let q = s.appsQueue, q > 0 { parts.append("\(q) downloading") }
        if let p = s.requestsPending, p > 0 { parts.append("\(p) request\(p == 1 ? "" : "s") pending") }
        return parts.isEmpty ? "idle" : parts.joined(separator: " · ")
    }

    private func mcColor(_ state: String) -> Color {
        switch state {
        case "running": return .green
        case "starting", "stopping": return .orange
        case "crashed": return .red
        default: return Color.secondary.opacity(0.5)
        }
    }
}

struct StatusDot: View {
    let status: PanelModel.Status
    var body: some View {
        Circle().fill(color).frame(width: 8, height: 8)
    }
    private var color: Color {
        switch status {
        case .ok: return .green
        case .degraded: return .orange
        case .unreachable, .authNeeded: return .red
        case .unconfigured: return .gray
        }
    }
}

struct MeterRow: View {
    let label: String
    let value: Double // 0…1
    let detail: String

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text(label).font(.system(size: 11, weight: .medium))
                    .foregroundStyle(.secondary)
                Spacer()
                Text(detail).font(.system(size: 11).monospacedDigit())
            }
            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule().fill(.quaternary)
                    Capsule()
                        .fill(barColor)
                        .frame(width: max(3, geo.size.width * min(value, 1)))
                }
            }
            .frame(height: 4)
        }
    }

    private var barColor: Color {
        if value >= 0.95 { return .red }
        if value >= 0.85 { return .orange }
        return .accentColor
    }
}
