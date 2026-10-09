// Lists the windows on screen (macOS), one per line: owner pid, layer, owner name, size. Window titles need the
// screen-recording permission and are not shown. Used by the CI scripts to count the settings window's browser
// windows and to find the menu bar icon (a window at layer 25).   usage: swift tools/ci/windows.swift
import CoreGraphics

let list = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as? [[String: Any]] ?? []
for w in list {
    let pid = w[kCGWindowOwnerPID as String] as? Int ?? 0
    let layer = w[kCGWindowLayer as String] as? Int ?? 0
    let owner = w[kCGWindowOwnerName as String] as? String ?? "?"
    let b = w[kCGWindowBounds as String] as? [String: Any] ?? [:]
    let width = b["Width"] as? Int ?? 0, height = b["Height"] as? Int ?? 0
    print("\(pid)\t\(layer)\t\(owner)\t\(width)x\(height)")
}
