package store

import (
	"path/filepath"
	"testing"
)

func TestConversationAndMessagesPersist(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "alice.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	conversation, err := database.CreateConversation()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AddMessage(conversation.ID, "user", "Hola Alice"); err != nil {
		t.Fatal(err)
	}
	if err := database.AddMessage(conversation.ID, "assistant", "Hola, ¿cómo estás?"); err != nil {
		t.Fatal(err)
	}

	messages, err := database.Messages(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Content != "Hola, ¿cómo estás?" {
		t.Fatalf("mensajes inesperados: %#v", messages)
	}
	conversations, err := database.Conversations()
	if err != nil {
		t.Fatal(err)
	}
	if conversations[0].Title != "Hola Alice" {
		t.Fatalf("título inesperado: %q", conversations[0].Title)
	}
}
