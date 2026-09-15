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

func TestManualTitleIsPersistentAndNotAutomaticallyOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alice.db")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	conversation, err := database.CreateConversation()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.RenameConversation(conversation.ID, "  Nuevo chat  "); err != nil {
		t.Fatal(err)
	}
	if err := database.AddMessage(conversation.ID, "user", "Esto no debe reemplazar el título"); err != nil {
		t.Fatal(err)
	}
	conversations, err := database.Conversations()
	if err != nil {
		t.Fatal(err)
	}
	if conversations[0].Title != "  Nuevo chat  " {
		t.Fatalf("el título manual fue reemplazado: %q", conversations[0].Title)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	conversations, err = database.Conversations()
	if err != nil {
		t.Fatal(err)
	}
	if conversations[0].Title != "  Nuevo chat  " {
		t.Fatalf("el título manual no sobrevivió a la reapertura: %q", conversations[0].Title)
	}
}
