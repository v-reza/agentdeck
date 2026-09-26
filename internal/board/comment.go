package board

// Comments — task discussion (ARCHITECTURE 3.16, 6.2.17, US-AD42).
//
// Four rules, each from the contract rather than from taste:
//
//  1. A comment is authored by a user or an agent, never both and never neither
//     (comments_author_chk). The HTTP surface only ever produces user-authored
//     rows today because the agent-facing route would be an Internal/Key Worker
//     endpoint and `api_keys` does not exist yet — but the domain accepts both so
//     the table's own constraint is the thing being respected, not the caller.
//
//  2. Writing is Member, reading is Viewer (US-AD42 AC2). That gate lives in the
//     route table, not here; this file is reachable from any handler.
//
//  3. Empty and over-long bodies are 400 (US-AD42 AC3). The contract asks for a
//     limit without naming one; MaxCommentBody is the decision and this is the
//     only place it is enforced.
//
//  4. Edit and delete are author-only. The SQL carries the author predicate as
//     well, so a bug here cannot edit another member's comment — and the answer
//     is 404 either way, because "not yours" and "not there" must be
//     indistinguishable to somebody probing ids.

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// CommentAuthor is who is speaking. Exactly one field is set.
type CommentAuthor struct {
	UserID  string
	AgentID string
}

// CreateComment appends a comment to a task and records the timeline event the
// events CHECK already reserves for it (`comment.created`).
func (s *Service) CreateComment(ctx context.Context, taskID, orgID, body string, author CommentAuthor) (Comment, error) {
	body, err := validateCommentBody(body)
	if err != nil {
		return Comment{}, err
	}
	if (author.UserID == "") == (author.AgentID == "") {
		// Both or neither. Refused here as well as in the CHECK so the caller
		// gets a 400 instead of a constraint violation surfacing as a 500.
		return Comment{}, ErrInvalidInput
	}
	task, err := s.repo.GetTask(ctx, taskID, orgID)
	if err != nil {
		return Comment{}, err
	}
	comment, err := s.repo.CreateComment(ctx, Comment{
		OrgID:         orgID,
		TaskID:        taskID,
		AuthorUserID:  author.UserID,
		AuthorAgentID: author.AgentID,
		Body:          body,
	})
	if err != nil {
		return Comment{}, err
	}
	// The event is written after the comment and its failure is not fatal: the
	// comment is the record that matters, and a timeline that is missing one
	// line is recoverable while a comment that was refused because its event
	// could not be written is not. `comment.created` is in the events CHECK
	// (ARCHITECTURE 3.15), so this cannot fail on an unknown kind.
	//
	// RecordTaskEvent takes bytes rather than a struct, so the payload is
	// marshalled here. A marshal failure is impossible for a map of a string and
	// an int64, and if it ever were, the comment still stands — hence the
	// discard rather than a rollback of a write the caller already has.
	if encoded, err := json.Marshal(commentEventPayload(comment)); err == nil {
		_, _ = s.RecordTaskEvent(ctx, orgID, task.BoardID, taskID, "comment.created", encoded)
	}
	return comment, nil
}

// ListComments answers 6.2.17's read. A task that is not in this workspace is
// 404 rather than an empty list — an empty list would confirm the id exists
// somewhere, which is the cross-tenant leak the repo's GetTask mapping closed.
func (s *Service) ListComments(ctx context.Context, taskID, orgID string) ([]Comment, error) {
	if _, err := s.repo.GetTask(ctx, taskID, orgID); err != nil {
		return nil, err
	}
	return s.repo.ListTaskComments(ctx, taskID, orgID)
}

// EditComment changes a comment's body. Only the author may, and the answer for
// "somebody else's" is the same 404 as "no such comment".
func (s *Service) EditComment(ctx context.Context, id int64, orgID, actorUserID, body string) (Comment, error) {
	body, err := validateCommentBody(body)
	if err != nil {
		return Comment{}, err
	}
	comment, err := s.repo.GetComment(ctx, id, orgID)
	if err != nil {
		return Comment{}, err
	}
	if comment.AuthorUserID == "" || comment.AuthorUserID != actorUserID {
		// Includes the agent-authored case: a comment nobody's session wrote is
		// not editable from a session. Deleting or editing a worker's note is an
		// operator action, not a member one, and there is no such route.
		return Comment{}, ErrNotFound
	}
	return s.repo.UpdateCommentBody(ctx, id, orgID, actorUserID, body)
}

// DeleteComment removes a comment. Author-only, same 404 shape as EditComment.
func (s *Service) DeleteComment(ctx context.Context, id int64, orgID, actorUserID string) error {
	comment, err := s.repo.GetComment(ctx, id, orgID)
	if err != nil {
		return err
	}
	if comment.AuthorUserID == "" || comment.AuthorUserID != actorUserID {
		return ErrNotFound
	}
	deleted, err := s.repo.DeleteComment(ctx, id, orgID, actorUserID)
	if err != nil {
		return err
	}
	if !deleted {
		// Lost a race with a concurrent delete. The row is gone, which is what
		// the caller asked for.
		return ErrNotFound
	}
	return nil
}

// validateCommentBody is US-AD42 AC3. The count is runes, not bytes: a 4096-rune
// Indonesian comment is longer in bytes, and refusing it for that would be a
// limit nobody could predict from the error message.
func validateCommentBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ErrInvalidInput
	}
	if utf8.RuneCountInString(body) > MaxCommentBody {
		return "", ErrInvalidInput
	}
	return body, nil
}

func commentEventPayload(c Comment) map[string]any {
	payload := map[string]any{
		"comment_id": c.ID,
		"excerpt":    excerpt(c.Body, 200),
	}
	if c.AuthorUserID != "" {
		payload["author_user_id"] = c.AuthorUserID
	} else {
		payload["author_agent_id"] = c.AuthorAgentID
	}
	return payload
}

// excerpt keeps the event payload small. N21 caps an event at 64 KB, and a
// timeline entry that quotes a whole comment verbatim would make the timeline
// the second copy of every comment.
func excerpt(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
