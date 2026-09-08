import CoreGraphics
import Foundation

// scroll X Y TICKS — scroll-wheel at a point; negative ticks scroll down.
// Needed because the page scroller is a div: PageDown/End do nothing.
let a = CommandLine.arguments
guard a.count >= 4, let x = Double(a[1]), let y = Double(a[2]), let ticks = Int32(a[3]) else {
    print("usage: scroll X Y TICKS")
    exit(1)
}
let src = CGEventSource(stateID: .hidSystemState)
CGEvent(mouseEventSource: src, mouseType: .mouseMoved, mouseCursorPosition: CGPoint(x: x, y: y), mouseButton: .left)?.post(tap: .cghidEventTap)
usleep(150_000)
let dir: Int32 = ticks < 0 ? -1 : 1
for _ in 0..<abs(ticks) {
    CGEvent(scrollWheelEvent2Source: src, units: .line, wheelCount: 1, wheel1: 3 * dir, wheel2: 0, wheel3: 0)?.post(tap: .cghidEventTap)
    usleep(30_000)
}
print("scrolled \(ticks) at \(Int(x)),\(Int(y))")
