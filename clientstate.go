package golake

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

func (c *APIClient) snapshotClientState() func() {
	querySeq := c.QuerySeq
	routeHint := c.routeHint
	nodeID := c.nodeID
	stateRestored := c.stateRestored
	sessionStateRaw := cloneRawMessage(c.sessionStateRaw)
	sessionState := cloneSessionState(c.sessionState)

	return func() {
		c.QuerySeq = querySeq
		c.routeHint = routeHint
		c.nodeID = nodeID
		c.stateRestored = stateRestored
		c.sessionStateRaw = sessionStateRaw
		c.sessionState = sessionState
	}
}

func cloneRawMessage(raw *json.RawMessage) *json.RawMessage {
	if raw == nil {
		return nil
	}
	cloned := json.RawMessage(append([]byte(nil), (*raw)...))
	return &cloned
}

func cloneSessionState(state *SessionState) *SessionState {
	if state == nil {
		return nil
	}

	cloned := *state
	if state.SecondaryRoles != nil {
		roles := append([]string(nil), (*state.SecondaryRoles)...)
		cloned.SecondaryRoles = &roles
	}
	if state.Settings != nil {
		settings := make(map[string]string, len(state.Settings))
		for key, value := range state.Settings {
			settings[key] = value
		}
		cloned.Settings = settings
	}
	return &cloned
}

type APIClientState struct {
	SessionID    string
	QuerySeq     int64
	RouteHint    string
	NodeID       string
	SessionState string
	Cookies      map[string]*http.Cookie
	Signature    []byte
}

var localStateSigningKey = func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("could not create Lake state signing key")
	}
	return key
}()

func (c *APIClient) signState(state APIClientState) []byte {
	state.Signature = nil
	credential := c.password
	if c.authMethod() == AuthMethodAccessToken {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		token, err := c.accessTokenLoader.LoadAccessToken(ctx, false)
		if err != nil || token == "" {
			return nil
		}
		credential = token
	}
	data, err := json.Marshal(struct {
		Endpoint, User, Tenant, Warehouse, Role, Credential string
		State                                               APIClientState
	}{c.apiEndpoint, c.user, c.tenant, c.warehouse, c.stateRole, credential, state})
	if err != nil {
		return nil
	}
	key := c.stateSigningKey
	if len(key) == 0 {
		key = localStateSigningKey
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

// WithState resumes a signed snapshot returned by GetState. An invalid state
// is reported on the next operation. Use RestoreState to handle it immediately.
func (c *APIClient) WithState(state *APIClientState) *APIClient {
	c.stateRestoreErr = c.RestoreState(state)
	return c
}

// RestoreState verifies the snapshot and its connection identity before changing the client.
func (c *APIClient) RestoreState(state *APIClientState) error {
	if state == nil {
		return nil
	}
	expected := c.signState(*state)
	if len(expected) == 0 || !hmac.Equal(expected, state.Signature) ||
		state.SessionID == "" || state.QuerySeq < 0 {
		return errors.New("invalid Lake client state or connection identity")
	}
	var parsed *SessionState
	var raw *json.RawMessage
	if state.SessionState != "" {
		rawState := json.RawMessage(state.SessionState)
		if err := json.Unmarshal(rawState, &parsed); err != nil || parsed == nil {
			return errors.New("invalid Lake session state")
		}
		raw = &rawState
	}
	for name, cookie := range state.Cookies {
		if cookie == nil || name != cookie.Name || cookie.Valid() != nil {
			return errors.New("invalid Lake session cookie")
		}
	}

	jar := NewIgnoreDomainCookieJar()
	for name, cookie := range state.Cookies {
		jar.SetCookies(nil, []*http.Cookie{{Name: name, Value: cookie.Value}})
	}

	c.SessionID = state.SessionID
	c.QuerySeq = state.QuerySeq
	c.routeHint = state.RouteHint
	c.nodeID = state.NodeID
	c.cli.Jar = jar
	if parsed != nil {
		c.sessionStateRaw = raw
		c.sessionState = parsed
	}
	c.stateRestored = true
	c.stateRestoreErr = nil
	return nil
}

func (c *APIClient) GetState() *APIClientState {
	var sessionStateStr string
	if c.sessionStateRaw != nil {
		sessionStateJson, _ := c.sessionStateRaw.MarshalJSON()
		sessionStateStr = string(sessionStateJson)
	}
	cookies := make(map[string]*http.Cookie)
	for _, cookie := range c.cli.Jar.Cookies(nil) {
		cookies[cookie.Name] = cookie
	}
	state := &APIClientState{
		SessionID:    c.SessionID,
		QuerySeq:     c.QuerySeq,
		RouteHint:    c.routeHint,
		NodeID:       c.nodeID,
		SessionState: sessionStateStr,
		Cookies:      cookies,
	}
	state.Signature = c.signState(*state)
	return state
}
