package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func facebookExchangeCode(ctx context.Context, appID, secret, redirect, code string) (string, error) {
	query := url.Values{"client_id": {appID}, "client_secret": {secret}, "redirect_uri": {redirect}, "code": {code}}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := facebookOAuthGet(ctx, facebookGraphBase+"/oauth/access_token?"+query.Encode(), &result); err != nil || result.AccessToken == "" {
		return "", errors.New("Facebook authorization code exchange failed")
	}
	return result.AccessToken, nil
}

func facebookLongLivedToken(ctx context.Context, appID, secret, userToken string) (string, error) {
	query := url.Values{
		"grant_type":        {"fb_exchange_token"},
		"client_id":         {appID},
		"client_secret":     {secret},
		"fb_exchange_token": {userToken},
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := facebookOAuthGet(ctx, facebookGraphBase+"/oauth/access_token?"+query.Encode(), &result); err != nil || result.AccessToken == "" {
		return "", errors.New("Facebook long-lived token exchange failed")
	}
	return result.AccessToken, nil
}

func facebookManagedPages(ctx context.Context, userToken string) ([]facebookOAuthPage, error) {
	query := url.Values{
		"fields":       {"id,name,access_token"},
		"limit":        {"100"},
		"access_token": {userToken},
	}
	var result struct {
		Data []facebookOAuthPage `json:"data"`
	}
	if err := facebookOAuthGet(ctx, facebookGraphBase+"/me/accounts?"+query.Encode(), &result); err != nil {
		return nil, err
	}
	pages := make([]facebookOAuthPage, 0, len(result.Data))
	for _, page := range result.Data {
		page.ID = strings.TrimSpace(page.ID)
		page.Name = strings.TrimSpace(page.Name)
		page.AccessToken = strings.TrimSpace(page.AccessToken)
		if facebookPageIDRe.MatchString(page.ID) && page.AccessToken != "" {
			if page.Name == "" {
				page.Name = "Facebook Page " + page.ID
			}
			pages = append(pages, page)
		}
	}
	return pages, nil
}

func facebookOAuthGet(ctx context.Context, target string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return errors.New("invalid Facebook OAuth request")
	}
	resp, err := facebookOAuthHTTP.Do(req)
	if err != nil {
		return errors.New("Facebook OAuth network request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Facebook OAuth HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(dst); err != nil {
		return errors.New("Facebook OAuth returned invalid JSON")
	}
	return nil
}
