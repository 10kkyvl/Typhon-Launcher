import CoreGraphics
import Foundation

// click X Y — posts a left click at screen coordinates (points, origin top-left).
let args = CommandLine.arguments
guard args.count >= 3, let x = Double(args[1]), let y = Double(args[2]) else {
    print("usage: click X Y")
    exit(1)
}
let point = CGPoint(x: x, y: y)
func post(_ type: CGEventType) {
    guard let ev = CGEvent(mouseEventSource: nil, mouseType: type, mouseCursorPosition: point, mouseButton: .left) else {
        print("failed to create event")
        exit(1)
    }
    ev.post(tap: .cghidEventTap)
}
post(.mouseMoved)
usleep(100_000)
post(.leftMouseDown)
usleep(80_000)
post(.leftMouseUp)
print("clicked \(Int(x)),\(Int(y))")
