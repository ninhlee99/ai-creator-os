package automation

import "github.com/ninhlee99/ai-creator-os/internal/network"

type fakeAccounts struct {
	accts []*network.Account
	err   error
}

func (f *fakeAccounts) List(statuses ...string) ([]*network.Account, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.accts, nil
}

func (f *fakeAccounts) Get(id int64) (*network.Account, error) {
	for _, a := range f.accts {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, nil
}

func (f *fakeAccounts) Transition(id int64, status string, extra map[string]any) (*network.Account, error) {
	return nil, nil
}

type fakeGate struct{ kill, dry bool }

func (g *fakeGate) KillSwitch() bool { return g.kill }
func (g *fakeGate) DryRun() bool     { return g.dry }

type fakeEnv map[string]string

func (e fakeEnv) EffectiveEnv(name string) (string, bool) {
	v, ok := e[name]
	return v, ok
}

