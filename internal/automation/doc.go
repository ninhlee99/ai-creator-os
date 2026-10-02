// Package automation holds the hands-off execution layer of AI Creator
// OS: the growth tick (plan -> production -> publish), the affiliate
// autopilot scheduler and the auto-publish hook. R2-W4 moved these out of
// the web dashboard so the schedule logic lives in one place with clean
// dependency interfaces — no web import anywhere in this package.
//
// Safety model (unchanged from the web era): the kill switch wins every
// tick, DRY-RUN gates every real action, and every external feed is
// fail-closed — without a real source nothing is written and the honest
// reason lands in the decision log.
package automation
