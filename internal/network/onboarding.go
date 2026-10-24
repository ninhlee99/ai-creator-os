package network

import (
	"context"
	"fmt"
)

// Onboarding pipeline: new account -> growing automatically. Live eligibility
// advances only after a verified TikTok follower count reaches the threshold.
//
//	onboarding -> researching -> persona_assigned -> growing
//
// The human adds the account and verifies its TikTok follower count when ready.
// Everything after is the pipeline. The research step is injectable
// (ResearchFunc) so tests and rehearsal can use a mock.

// TopicBrief is the output of one research run for an account.
type TopicBrief struct {
	Niche  string
	Topics []string
	Source string // "hint" | "auto" | "fallback"
	Hint   string
}

// ResearchFunc resolves an account's niche and TikTok video topics.
// The hint argument is advisory; the implementation re-reads the account's
// stored niche_hint, mirroring Python's _research.
type ResearchFunc func(hint string) (TopicBrief, error)

// MakeTopicResearch builds a ResearchFunc backed by the topic engine.
//
// It honors the account's niche_hint when set (source="hint" — the hint is a
// soft suggestion, never a final decision); otherwise the model
// auto-researches a niche via the LLM client, avoiding niches already taken
// by other accounts (source="auto"); with no LLM it falls back to the
// persona default (source="fallback").
func MakeTopicResearch(mgr *AccountManager, accountID int64, llm LLMClient) ResearchFunc {
	return func(hint string) (TopicBrief, error) {
		acct, err := mgr.Get(accountID)
		if err != nil {
			return TopicBrief{}, err
		}
		others, err := mgr.List()
		if err != nil {
			return TopicBrief{}, err
		}
		var taken []string
		for _, a := range others {
			if a.ID != accountID && a.Niche != "" {
				taken = append(taken, a.Niche)
			}
		}
		plan := ResolveTopic(context.Background(), llm, acct.NicheHint, acct.Persona, taken)
		if _, err := mgr.SetTopicPlan(accountID, plan.Niche, plan.Topics); err != nil {
			return TopicBrief{}, err
		}
		return TopicBrief{
			Niche:  plan.Niche,
			Topics: plan.Topics,
			Source: plan.Source,
			Hint:   acct.NicheHint,
		}, nil
	}
}

// OnboardStep advances one account by exactly one pipeline stage and
// returns the new status. Idempotent per stage. Uses the real topic engine
// with a nil LLM (fallback research) when no research function is injected;
// see OnboardStepWithLLM to supply one.
func OnboardStep(mgr *AccountManager, accountID int64) (string, error) {
	return onboardStep(mgr, accountID, nil, nil)
}

// OnboardStepWithLLM is OnboardStep with an LLM client for the research
// stage (nil still allowed: falls back to hint/persona defaults).
func OnboardStepWithLLM(mgr *AccountManager, accountID int64, llm LLMClient) (string, error) {
	return onboardStep(mgr, accountID, nil, llm)
}

func onboardStep(mgr *AccountManager, accountID int64, research ResearchFunc, llm LLMClient) (string, error) {
	acct, err := mgr.Get(accountID)
	if err != nil {
		return "", err
	}

	switch acct.Status {
	case "onboarding":
		if research == nil {
			research = MakeTopicResearch(mgr, accountID, llm)
		}
		brief, err := research(acct.NicheHint)
		if err != nil {
			return "", err
		}
		target := acct.Username
		if err := mgr.ledger.Decide("onboarding", "niche_research", &target,
			"đã chốt chủ đề (nguồn: "+brief.Source+")",
			map[string]any{
				"brief": map[string]any{
					"niche":  brief.Niche,
					"topics": brief.Topics,
					"source": brief.Source,
					"hint":   brief.Hint,
				},
				"hint": acct.NicheHint,
			}); err != nil {
			return "", err
		}
		next, err := mgr.Transition(accountID, "researching", nil)
		if err != nil {
			return "", err
		}
		return next.Status, nil

	case "researching":
		// The persona is chosen to FIT the researched niche. The hint was
		// only a soft suggestion; the niche resolved in the research step is
		// NEVER overwritten by the persona label — only the persona field
		// is written.
		nicheForMatch := firstNonEmpty(acct.Niche, acct.NicheHint)
		persona := AssignPersona(nicheForMatch, nil)
		p, _ := Describe(persona)
		target := acct.Username
		if err := mgr.ledger.Decide("onboarding", "persona_assigned", &target,
			fmt.Sprintf("gán tính cách %s cho chủ đề “%s”", p.Label, acct.Niche),
			map[string]any{
				"persona":          persona,
				"niche":            acct.Niche,
				"affiliate_niches": p.AffiliateNiches,
				"red_lines":        p.RedLines,
			}); err != nil {
			return "", err
		}
		next, err := mgr.Transition(accountID, "persona_assigned",
			map[string]any{"persona": persona})
		if err != nil {
			return "", err
		}
		return next.Status, nil

	case "persona_assigned":
		// The content factory grows the account with short videos.
		next, err := mgr.Transition(accountID, "growing", nil)
		if err != nil {
			return "", err
		}
		return next.Status, nil

	case "growing":
		return "growing", nil // account remains ready for TikTok video flows

	default:
		return acct.Status, nil // paused/penalized/retired: pipeline done
	}
}
