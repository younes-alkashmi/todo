package api

import (
	"cloud.google.com/go/firestore"
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"time"
)

type PushMutation struct {
	MutationID  string                 `json:"mutationId"`
	EntityType  string                 `json:"entityType"`
	EntityID    string                 `json:"entityId"`
	BaseVersion int                    `json:"baseVersion"`
	Operation   string                 `json:"operation"`
	Payload     map[string]interface{} `json:"payload"`
}

func Push(c *gin.Context, fs *firestore.Client) {
	uid := UID(c)
	var req struct {
		Mutations []PushMutation `json:"mutations"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	results := make([]gin.H, 0, len(req.Mutations))
	for _, m := range req.Mutations {
		result := applyMutation(c.Request.Context(), fs, uid, m)
		results = append(results, result)
	}
	c.JSON(200, gin.H{"results": results})
}

func applyMutation(ctx context.Context, fs *firestore.Client, uid string, m PushMutation) gin.H {
	receiptRef := fs.Collection("mutation_receipts").Doc(m.MutationID)
	taskRef := fs.Collection("users").Doc(uid).Collection("tasks").Doc(m.EntityID)

	var out gin.H
	err := fs.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// 1. Idempotency: receipt exists → already applied
		if _, err := tx.Get(receiptRef); err == nil {
			out = gin.H{"mutationId": m.MutationID, "status": "accepted", "note": "replayed"}
			return nil
		} else if status.Code(err) != codes.NotFound {
			return err
		}

		// 2. Conflict check (create has no prior version)
		if m.Operation != "create" {
			snap, err := tx.Get(taskRef)
			if err != nil {
				out = gin.H{"mutationId": m.MutationID, "status": "denied", "reason": "entity not found"}
				return nil
			}
			data := snap.Data()
			curVersion, _ := data["version"].(int64)
			if int(curVersion) != m.BaseVersion {
				out = gin.H{
					"mutationId":    m.MutationID,
					"status":        "conflict",
					"serverVersion": curVersion,
					"serverPayload": data,
				}
				return nil
			}
		}

		// 3. Apply
		payload := normalizePayload(m.Payload)
		payload["version"] = int64(m.BaseVersion + 1)
		payload["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		if err := tx.Set(taskRef, payload); err != nil {
			return err
		}
		if err := tx.Set(receiptRef, map[string]interface{}{
			"uid":       uid,
			"entityId":  m.EntityID,
			"appliedAt": time.Now().UTC(),
		}); err != nil {
			return err
		}
		out = gin.H{"mutationId": m.MutationID, "status": "accepted", "version": payload["version"]}
		return nil
	})
	if err != nil {
		return gin.H{"mutationId": m.MutationID, "status": "denied", "reason": err.Error()}
	}
	return out
}

func Pull(c *gin.Context, fs *firestore.Client) {
	uid := UID(c)
	cursorStr := c.Query("cursor")
	cursor := 0
	if cursorStr != "" {
		_, _ = fmt.Sscanf(cursorStr, "%d", &cursor)
	}

	iter := fs.Collection("users").Doc(uid).Collection("tasks").
		Where("version", ">", int64(cursor)).
		OrderBy("version", firestore.Asc).
		Limit(100).
		Documents(c.Request.Context())
	defer iter.Stop()

	var changes []map[string]interface{}
	lastVersion := int64(cursor)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		d := doc.Data()
		d["id"] = doc.Ref.ID
		changes = append(changes, d)
		if v, ok := d["version"].(int64); ok {
			lastVersion = v
		}
	}
	c.JSON(200, gin.H{"changes": changes, "nextCursor": lastVersion})
}

func normalizePayload(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		switch k {
		case "ID":
			out["id"] = v
		case "OwnerUID":
			out["ownerUid"] = v
		case "ProjectID":
			out["projectId"] = v
		case "ParentID":
			out["parentId"] = v
		case "Title":
			out["title"] = v
		case "Description":
			out["description"] = v
		case "DueDate":
			out["dueDate"] = v
		case "Tags":
			out["tags"] = v
		case "Completed":
			out["completed"] = v
		case "Version":
			out["version"] = v
		case "CreatedAt":
			out["createdAt"] = v
		case "UpdatedAt":
			out["updatedAt"] = v
		case "DeletedAt":
			out["deletedAt"] = v
		default:
			out[k] = v
		}
	}
	return out
}
