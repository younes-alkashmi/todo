package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/younes-alkashmi/todo/internal/store"
)

type Client struct {
	apiBase string
	token   string
	store   *store.Store
	http    *http.Client
}

func New(apiBase, token string, st *store.Store) *Client {
	return &Client{
		apiBase: apiBase,
		token:   token,
		store:   st,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

type pushMutation struct {
	MutationID  string                 `json:"mutationId"`
	EntityType  string                 `json:"entityType"`
	EntityID    string                 `json:"entityId"`
	BaseVersion int                    `json:"baseVersion"`
	Operation   string                 `json:"operation"`
	Payload     map[string]interface{} `json:"payload"`
}

type pushResult struct {
	MutationID    string                 `json:"mutationId"`
	Status        string                 `json:"status"` // accepted|conflict|denied
	Version       int64                  `json:"version,omitempty"`
	ServerVersion int64                  `json:"serverVersion,omitempty"`
	ServerPayload map[string]interface{} `json:"serverPayload,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
}

func (c *Client) SyncOnce(ctx context.Context) error {
	// 1. Push pending mutations
	pending, err := c.store.Tasks().PendingMutations(ctx)
	if err != nil {
		return fmt.Errorf("read outbox: %w", err)
	}

	if len(pending) > 0 {
		batch := make([]pushMutation, 0, len(pending))
		for _, m := range pending {
			if m.State == "conflict" {
				continue
			}
			var payload map[string]interface{}
			_ = json.Unmarshal(m.Payload, &payload)
			batch = append(batch, pushMutation{
				MutationID:  m.MutationID,
				EntityType:  m.EntityType,
				EntityID:    m.EntityID,
				BaseVersion: m.BaseVersion,
				Operation:   m.Operation,
				Payload:     payload,
			})
		}

		if len(batch) > 0 {
			results, err := c.postPush(ctx, batch)
			if err != nil {
				return fmt.Errorf("push: %w", err)
			}

			for _, r := range results {
				switch r.Status {
				case "accepted":
					_ = c.store.Tasks().MarkMutationSent(ctx, r.MutationID)
				case "conflict":
					_ = c.store.Tasks().MarkConflict(ctx, r.MutationID, r.ServerPayload)
					// Do NOT retry; wait for user resolution
				case "denied":
					_ = c.store.Tasks().MarkDenied(ctx, r.MutationID, r.Reason)
				}
			}
		}
	}

	// 2. Pull remote changes since cursor
	cursor, _ := c.store.Meta().Get("sync_cursor")
	changes, next, err := c.pullChanges(ctx, cursor)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	if len(changes) > 0 || next != cursor {
		if err := c.store.Tasks().ApplyRemoteChanges(ctx, changes, next); err != nil {
			return fmt.Errorf("apply remote: %w", err)
		}
	}

	return nil
}

func (c *Client) postPush(ctx context.Context, batch []pushMutation) ([]pushResult, error) {
	body, _ := json.Marshal(map[string]interface{}{"mutations": batch})
	req, _ := http.NewRequestWithContext(ctx, "POST", c.apiBase+"/sync/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var out struct {
		Results []pushResult `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

func (c *Client) pullChanges(ctx context.Context, cursor string) ([]map[string]interface{}, string, error) {
	url := c.apiBase + "/sync/pull"
	if cursor != "" {
		url += "?cursor=" + cursor
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, cursor, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, cursor, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var out struct {
		Changes    []map[string]interface{} `json:"changes"`
		NextCursor interface{}              `json:"nextCursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, cursor, err
	}

	// nextCursor is int64 from Firestore; convert to string
	next := fmt.Sprintf("%v", out.NextCursor)
	if out.NextCursor == nil {
		next = cursor
	}
	return out.Changes, next, nil
}
