package automation

// MasterSwitchGate lets the network daemon read the persisted master
// switch live from the single settings facade (R2-W4, R2-08). The static
// NetConfig.MasterSwitch field is only the startup seed; the gate below
// decides every tick, so the UI toggle takes effect without a restart.
type MasterSwitchGate struct {
	Settings Settings
}

// MasterEnabled reports the persisted switch; nil settings = OFF
// (fail-closed — an unwired gate can never start the daemon).
func (g MasterSwitchGate) MasterEnabled() bool {
	return MasterOn(g.Settings)
}
