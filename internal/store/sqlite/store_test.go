package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/store"
)

func TestStoreCreatesOrganizationChannelAndMessage(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UTC()
	user := domain.User{ID: "usr_1", DisplayName: "Admin", CreatedAt: now}
	org := domain.Organization{ID: "org_1", Name: "Default", CreatedAt: now}
	channel := domain.Channel{ID: "chn_1", OrganizationID: org.ID, Name: "general", CreatedAt: now}
	message := domain.Message{
		ID: "msg_1", OrganizationID: org.ID, ConversationType: domain.ConversationChannel,
		ConversationID: channel.ID, SenderType: domain.SenderUser, SenderID: user.ID,
		Kind: domain.MessageText, Body: "hello", CreatedAt: now,
	}

	err = st.Tx(ctx, func(tx store.Tx) error {
		if err := tx.Users().Create(ctx, user); err != nil {
			return err
		}
		if err := tx.Organizations().Create(ctx, org); err != nil {
			return err
		}
		if err := tx.Organizations().AddMember(ctx, org.ID, user.ID, domain.RoleOwner); err != nil {
			return err
		}
		if err := tx.Channels().Create(ctx, channel); err != nil {
			return err
		}
		return tx.Messages().Create(ctx, message)
	})
	if err != nil {
		t.Fatal(err)
	}

	channels, err := st.Channels().ListByOrganization(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 || channels[0].Name != "general" {
		t.Fatalf("channels = %#v", channels)
	}

	messages, err := st.Messages().List(ctx, domain.ConversationChannel, channel.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Body != "hello" {
		t.Fatalf("messages = %#v", messages)
	}
}
