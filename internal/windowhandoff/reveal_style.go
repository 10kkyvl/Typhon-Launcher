package windowhandoff

import "errors"

// exLayered is WS_EX_LAYERED. The decision below is kept apart from the
// Win32 calls so that it can be tested on every platform.
const exLayered uint32 = 0x00080000

// styleOps are the window calls a Windows reveal makes.
type styleOps interface {
	exStyle() uint32
	setExStyle(style uint32)
	setAlpha(alpha byte) error
	setTransitions(disabled bool) error
	frameChanged() error
}

// revealStyled makes a window that paintHidden left invisible opaque again,
// puts back the extended style it had before (saved, when paintHidden saved
// it) and re-enables the system animations. A window shown plainly instead is
// left visible as it is, with its animations back.
func revealStyled(ops styleOps, saved uint32, haveSaved bool, hidden uint32) error {
	orig := saved
	if !haveSaved {
		orig = ops.exStyle() &^ hidden
	}
	var errs []error
	// A window that is not layered was shown plainly (the Preparer's
	// fallback) and is already visible; alpha only exists on a layered one.
	if ops.exStyle()&exLayered != 0 {
		if err := ops.setAlpha(255); err != nil {
			errs = append(errs, err)
		}
	}
	// Dropping WS_EX_LAYERED (unless the app had it) makes the window an
	// ordinary one again; the others are restored from the saved style.
	ops.setExStyle(orig)
	if err := ops.setTransitions(false); err != nil {
		errs = append(errs, err)
	}
	// Repaint the frame after the style change.
	if err := ops.frameChanged(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
