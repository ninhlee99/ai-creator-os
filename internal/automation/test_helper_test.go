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
