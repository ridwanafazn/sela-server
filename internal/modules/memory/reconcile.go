package memory

import (
	"context"
	"time"

	"gorm.io/gorm"
)

const eligibleSourceCountSQL = `(
	SELECT count(*)
	FROM memory_sources ms
	LEFT JOIN journals j ON j.id = ms.journal_id
	LEFT JOIN messages msg ON msg.id = ms.message_id
	LEFT JOIN conversations c ON c.id = msg.conversation_id
	WHERE ms.memory_id = m.id
	  AND (
	    (ms.journal_id IS NOT NULL AND j.status = 'active' AND j.deleted_at IS NULL)
	    OR (ms.message_id IS NOT NULL AND msg.id IS NOT NULL AND c.deleted_at IS NULL)
	  )
)`

// ReconcileJournalSourcesTx recomputes memory source counts after a journal
// changes status or is deleted. Memories that lose every eligible source
// return to candidate status for review.
func ReconcileJournalSourcesTx(ctx context.Context, tx *gorm.DB, userID, journalID string, now time.Time) error {
	return tx.WithContext(ctx).Exec(
		`UPDATE personal_memories m
		 SET source_count = `+eligibleSourceCountSQL+`,
		     status = CASE
		       WHEN `+eligibleSourceCountSQL+` = 0 AND m.status = 'active' THEN 'candidate'
		       ELSE m.status
		     END,
		     updated_at = ?
		 WHERE m.user_id = ? AND m.deleted_at IS NULL
		   AND EXISTS (
		     SELECT 1 FROM memory_sources ms
		     WHERE ms.memory_id = m.id AND ms.journal_id = ?
		   )`,
		now, userID, journalID,
	).Error
}

// ReconcileConversationSourcesTx recomputes memory source counts after a
// conversation is deleted. Messages from a deleted conversation are no longer
// eligible provenance.
func ReconcileConversationSourcesTx(ctx context.Context, tx *gorm.DB, userID, conversationID string, now time.Time) error {
	return tx.WithContext(ctx).Exec(
		`UPDATE personal_memories m
		 SET source_count = `+eligibleSourceCountSQL+`,
		     status = CASE
		       WHEN `+eligibleSourceCountSQL+` = 0 AND m.status = 'active' THEN 'candidate'
		       ELSE m.status
		     END,
		     updated_at = ?
		 WHERE m.user_id = ? AND m.deleted_at IS NULL
		   AND EXISTS (
		     SELECT 1
		     FROM memory_sources ms
		     JOIN messages msg ON msg.id = ms.message_id
		     WHERE ms.memory_id = m.id AND msg.conversation_id = ?
		   )`,
		now, userID, conversationID,
	).Error
}
