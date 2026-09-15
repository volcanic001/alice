package store

import (
	"database/sql"
	"fmt"

	"github.com/volcanic001/alice/internal/chat"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id INTEGER PRIMARY KEY, title TEXT NOT NULL DEFAULT 'Nuevo chat',
			manual_title INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY, conversation_id INTEGER NOT NULL,
			role TEXT NOT NULL, content TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
		)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("inicializar SQLite: %w", err)
		}
	}
	if err := ensureManualTitleColumn(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrar SQLite: %w", err)
	}
	return &Store{db: db}, nil
}

func ensureManualTitleColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(conversations)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == "manual_title" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE conversations ADD COLUMN manual_title INTEGER NOT NULL DEFAULT 0`)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Conversations() ([]chat.Conversation, error) {
	rows, err := s.db.Query(`SELECT id, title, updated_at FROM conversations ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var conversations []chat.Conversation
	for rows.Next() {
		var conversation chat.Conversation
		if err := rows.Scan(&conversation.ID, &conversation.Title, &conversation.UpdatedAt); err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
	}
	return conversations, rows.Err()
}

func (s *Store) CreateConversation() (chat.Conversation, error) {
	result, err := s.db.Exec(`INSERT INTO conversations DEFAULT VALUES`)
	if err != nil {
		return chat.Conversation{}, err
	}
	id, err := result.LastInsertId()
	return chat.Conversation{ID: id, Title: "Nuevo chat"}, err
}

func (s *Store) DeleteConversation(conversationID int64) error {
	_, err := s.db.Exec(`DELETE FROM conversations WHERE id=?`, conversationID)
	return err
}

// RenameConversation records a user-chosen title without changing the history
// ordering. A manual title is never replaced by automatic title generation.
func (s *Store) RenameConversation(conversationID int64, title string) error {
	_, err := s.db.Exec(`UPDATE conversations SET title=?, manual_title=1 WHERE id=?`, title, conversationID)
	return err
}

func (s *Store) Messages(conversationID int64) ([]chat.Message, error) {
	rows, err := s.db.Query(`SELECT id, conversation_id, role, content FROM messages WHERE conversation_id=? ORDER BY id`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []chat.Message
	for rows.Next() {
		var message chat.Message
		if err := rows.Scan(&message.ID, &message.ConversationID, &message.Role, &message.Content); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (s *Store) AddMessage(conversationID int64, role, content string) error {
	transaction, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err = transaction.Exec(`INSERT INTO messages(conversation_id, role, content) VALUES(?,?,?)`, conversationID, role, content); err != nil {
		return err
	}
	if role == "user" {
		if _, err = transaction.Exec(`UPDATE conversations SET title=CASE WHEN manual_title=0 AND title='Nuevo chat' THEN substr(?,1,48) ELSE title END, updated_at=CURRENT_TIMESTAMP WHERE id=?`, content, conversationID); err != nil {
			return err
		}
	} else {
		if _, err = transaction.Exec(`UPDATE conversations SET updated_at=CURRENT_TIMESTAMP WHERE id=?`, conversationID); err != nil {
			return err
		}
	}
	return transaction.Commit()
}
