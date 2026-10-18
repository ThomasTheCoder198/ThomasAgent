package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	convrepo "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/repository"
	convservice "github.com/thomasthecoder198/thomastheragx/backend-core/internal/conversation/service"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/config"
	"github.com/thomasthecoder198/thomastheragx/backend-core/internal/platform/postgres/pgtest"
)

func TestConversationRetentionAcrossTenantsAndActiveRuns(t *testing.T) {
	pool := pgtest.Start(t)
	now := time.Now().UTC()
	ids := make([]uuid.UUID, 0, 7)
	for _, org := range []string{"retention-a", "retention-b"} {
		_, err := pool.Exec(t.Context(), `INSERT INTO organizations (id, tenant_id, name, external_parsing) VALUES ($1, 'default', $1, true)`, org)
		require.NoError(t, err)
		for userIndex := range 2 {
			userID, agentID := uuid.New(), uuid.New()
			_, err = pool.Exec(t.Context(), `INSERT INTO users (id, tenant_id, email, password_hash) VALUES ($1,$2,$3,'hash')`, userID, org, uuid.NewString()+"@example.com")
			require.NoError(t, err)
			_, err = pool.Exec(t.Context(), `INSERT INTO agents (id, tenant_id, name, kb_id) VALUES ($1,$2,$3,$4)`, agentID, org, uuid.NewString(), uuid.New())
			require.NoError(t, err)
			for age := range 2 {
				id := uuid.New()
				idleDays := 31
				if age == 1 {
					idleDays = 29
				}
				_, err = pool.Exec(t.Context(), `INSERT INTO conversations (id,tenant_id,agent_id,user_id,updated_at) VALUES ($1,$2,$3,$4,$5)`, id, org, agentID, userID, now.Add(-time.Duration(idleDays)*24*time.Hour))
				require.NoError(t, err)
				if age == 0 {
					ids = append(ids, id)
				}
			}
			if userIndex == 0 {
				id := uuid.New()
				_, err = pool.Exec(t.Context(), `INSERT INTO conversations (id,tenant_id,agent_id,user_id,updated_at) VALUES ($1,$2,$3,$4,$5)`, id, org, agentID, userID, now.Add(-31*24*time.Hour))
				require.NoError(t, err)
				_, err = pool.Exec(t.Context(), `INSERT INTO runs (id,tenant_id,conversation_id,assistant_message_id,status,deadline_at) VALUES ($1,$2,$3,'a','running',$4)`, uuid.New(), org, id, now.Add(time.Hour))
				require.NoError(t, err)
			}
		}
	}
	// A completed run and its step, plus a message, must cascade with the stale parent.
	var org string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT tenant_id FROM conversations WHERE id=$1`, ids[0]).Scan(&org))
	_, err := pool.Exec(t.Context(), `INSERT INTO messages (conversation_id,id,tenant_id,seq,role,parts) VALUES ($1,'u',$2,1,'user','[]')`, ids[0], org)
	require.NoError(t, err)
	runID := uuid.New()
	_, err = pool.Exec(t.Context(), `INSERT INTO runs (id,tenant_id,conversation_id,assistant_message_id,status,deadline_at) VALUES ($1,$2,$3,'a','completed',$4)`, runID, org, ids[0], now)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO run_steps (run_id,step,tenant_id,kind,outcome,duration_ms,started_at) VALUES ($1,1,$2,'model','success',1,$3)`, runID, org, now)
	require.NoError(t, err)
	var touchedOrg string
	var touchedUser uuid.UUID
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT tenant_id,user_id FROM conversations WHERE id=$1`, ids[1]).Scan(&touchedOrg, &touchedUser))
	require.NoError(t, newChatStore(pool).AppendUserMessage(conversationContext(touchedOrg, touchedUser), ids[1], userText("touch", "recent activity")))
	service := convservice.NewRetentionService(pool, convrepo.NewConversationRepo(), config.ConversationConfig{Retention: 30 * 24 * time.Hour, RetentionPurgeBatch: 1}, func() time.Time { return now })
	count, err := service.PurgeInactiveConversations(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 3, count)
	var remaining int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM conversations`).Scan(&remaining))
	require.Equal(t, 7, remaining) // four recent, one touched, and two active-run rows
	for _, table := range []string{"messages", "runs", "run_steps"} {
		require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM `+table+` WHERE `+map[string]string{"messages": "conversation_id", "runs": "conversation_id", "run_steps": "run_id"}[table]+`=$1`, map[string]uuid.UUID{"messages": ids[0], "runs": ids[0], "run_steps": runID}[table]).Scan(&remaining))
		require.Zero(t, remaining)
	}
}
