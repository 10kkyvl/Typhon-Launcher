import CoreGraphics
import Foundation

// Prints every window whose owner contains "typhon": id, pid, name, bounds.
// Pass "all" to include windows on other Spaces (screencapture -l still works
// for those, clicks do not).
let all = CommandLine.arguments.dropFirst().first == "all"
let opts: CGWindowListOption = all ? [.optionAll, .excludeDesktopElements] : [.optionOnScreenOnly, .excludeDesktopElements]
guard let list = CGWindowListCopyWindowInfo(opts, kCGNullWindowID) as? [[String: AnyObject]] else {
    print("no window list")
    exit(1)
}
for w in list {
    let owner = (w[kCGWindowOwnerName as String] as? String) ?? ""
    guard owner.lowercased().contains("typhon") else { continue }
    let wid = w[kCGWindowNumber as String] as? Int ?? -1
    let pid = w[kCGWindowOwnerPID as String] as? Int ?? -1
    let layer = w[kCGWindowLayer as String] as? Int ?? -1
    let name = (w[kCGWindowName as String] as? String) ?? ""
    let b = w[kCGWindowBounds as String] as? [String: CGFloat] ?? [:]
    print("id=\(wid) pid=\(pid) layer=\(layer) name=\(name) x=\(Int(b["X"] ?? 0)) y=\(Int(b["Y"] ?? 0)) w=\(Int(b["Width"] ?? 0)) h=\(Int(b["Height"] ?? 0))")
}
