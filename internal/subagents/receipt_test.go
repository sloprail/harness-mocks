package subagents

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReceipt(t *testing.T) {
	assert.Equal(t, "a1", ReceiptAgentID(`{"agent_id":"a1","nickname":"James"}`))
	assert.Empty(t, ReceiptAgentID(`{"status":{}}`))
	assert.Empty(t, ReceiptAgentID("not json"))
	assert.JSONEq(t, `{"agent_id":"a1","nickname":"<NICKNAME>"}`, ReceiptWithoutNickname(`{"agent_id":"a1","nickname":"James"}`))
	assert.Equal(t, `{"status":{}}`, ReceiptWithoutNickname(`{"status":{}}`))
	assert.Equal(t, "plain", ReceiptWithoutNickname("plain"))
}
