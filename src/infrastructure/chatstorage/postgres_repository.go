package chatstorage

import (
    "context"
    "database/sql"
    "fmt"
    "strings"
    "time"

    domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
    "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
    "github.com/sirupsen/logrus"
    "go.mau.fi/whatsmeow/types"
    "go.mau.fi/whatsmeow/types/events"
)

type PostgresRepository struct {
    db *sql.DB
}

func NewPostgresRepository(db *sql.DB) domainChatStorage.IChatStorageRepository {
    return &PostgresRepository{db: db}
}

func (r *PostgresRepository) StoreChat(chat *domainChatStorage.Chat) error {
    query := `
        INSERT INTO chats (jid, name, last_message_time, ephemeral_expiration)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (jid) DO UPDATE SET
            name = EXCLUDED.name,
            last_message_time = EXCLUDED.last_message_time,
            ephemeral_expiration = EXCLUDED.ephemeral_expiration,
            updated_at = CURRENT_TIMESTAMP
    `
    _, err := r.db.Exec(query, chat.JID, chat.Name, chat.LastMessageTime, chat.EphemeralExpiration)
    return err
}

func (r *PostgresRepository) GetChat(jid string) (*domainChatStorage.Chat, error) {
    row := r.db.QueryRow(`
        SELECT jid, name, last_message_time, ephemeral_expiration, created_at, updated_at
        FROM chats WHERE jid = $1
    `, jid)
    return r.scanChat(row)
}

// DeleteChat removes a chat (messages will be removed via FK constraints if set)
func (r *PostgresRepository) DeleteChat(jid string) error {
    _, err := r.db.Exec(`DELETE FROM chats WHERE jid = $1`, jid)
    return err
}

// GetChats returns a list of chats with optional filters and pagination
func (r *PostgresRepository) GetChats(filter *domainChatStorage.ChatFilter) ([]*domainChatStorage.Chat, error) {
    base := `SELECT jid, name, last_message_time, ephemeral_expiration, created_at, updated_at FROM chats c`
    var where []string
    var args []any

    if filter != nil {
        if filter.SearchName != "" {
            where = append(where, "c.name ILIKE $"+fmt.Sprint(len(args)+1))
            args = append(args, "%"+filter.SearchName+"%")
        }
        if filter.HasMedia {
            where = append(where, "EXISTS (SELECT 1 FROM messages m WHERE m.chat_jid = c.jid AND m.media_type <> '')")
        }
    }

    if len(where) > 0 {
        base += " WHERE " + strings.Join(where, " AND ")
    }

    base += " ORDER BY last_message_time DESC"

    if filter != nil {
        if filter.Limit > 0 {
            base += " LIMIT $" + fmt.Sprint(len(args)+1)
            args = append(args, filter.Limit)
        }
        if filter.Offset > 0 {
            base += " OFFSET $" + fmt.Sprint(len(args)+1)
            args = append(args, filter.Offset)
        }
    }

    rows, err := r.db.Query(base, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var chats []*domainChatStorage.Chat
    for rows.Next() {
        c, err := r.scanChat(rows)
        if err != nil {
            return nil, err
        }
        chats = append(chats, c)
    }
    return chats, rows.Err()
}

func (r *PostgresRepository) StoreMessage(message *domainChatStorage.Message) error {
    query := `
        INSERT INTO messages (
            id, chat_jid, sender, content, timestamp, is_from_me,
            media_type, filename, url, media_key, file_sha256,
            file_enc_sha256, file_length
        ) VALUES (
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
        )
        ON CONFLICT (id, chat_jid) DO UPDATE SET
            sender = EXCLUDED.sender,
            content = EXCLUDED.content,
            timestamp = EXCLUDED.timestamp,
            is_from_me = EXCLUDED.is_from_me,
            media_type = EXCLUDED.media_type,
            filename = EXCLUDED.filename,
            url = EXCLUDED.url,
            media_key = EXCLUDED.media_key,
            file_sha256 = EXCLUDED.file_sha256,
            file_enc_sha256 = EXCLUDED.file_enc_sha256,
            file_length = EXCLUDED.file_length,
            updated_at = CURRENT_TIMESTAMP
    `
    _, err := r.db.Exec(query,
        message.ID, message.ChatJID, message.Sender, message.Content, message.Timestamp, message.IsFromMe,
        message.MediaType, message.Filename, message.URL, message.MediaKey, message.FileSHA256, message.FileEncSHA256, message.FileLength,
    )
    return err
}

func (r *PostgresRepository) StoreMessagesBatch(messages []*domainChatStorage.Message) error {
    tx, err := r.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    stmt, err := tx.Prepare(`
        INSERT INTO messages (
            id, chat_jid, sender, content, timestamp, is_from_me,
            media_type, filename, url, media_key, file_sha256,
            file_enc_sha256, file_length
        ) VALUES (
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
        )
        ON CONFLICT (id, chat_jid) DO UPDATE SET
            sender = EXCLUDED.sender,
            content = EXCLUDED.content,
            timestamp = EXCLUDED.timestamp,
            is_from_me = EXCLUDED.is_from_me,
            media_type = EXCLUDED.media_type,
            filename = EXCLUDED.filename,
            url = EXCLUDED.url,
            media_key = EXCLUDED.media_key,
            file_sha256 = EXCLUDED.file_sha256,
            file_enc_sha256 = EXCLUDED.file_enc_sha256,
            file_length = EXCLUDED.file_length,
            updated_at = CURRENT_TIMESTAMP
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, m := range messages {
        if _, err := stmt.Exec(
            m.ID, m.ChatJID, m.Sender, m.Content, m.Timestamp, m.IsFromMe,
            m.MediaType, m.Filename, m.URL, m.MediaKey, m.FileSHA256, m.FileEncSHA256, m.FileLength,
        ); err != nil {
            return err
        }
    }

    return tx.Commit()
}

// GetMessageByID retrieves a single message by ID
func (r *PostgresRepository) GetMessageByID(id string) (*domainChatStorage.Message, error) {
    row := r.db.QueryRow(`
        SELECT id, chat_jid, sender, content, timestamp, is_from_me,
               media_type, filename, url, media_key, file_sha256,
               file_enc_sha256, file_length, created_at, updated_at
        FROM messages WHERE id = $1
        ORDER BY timestamp DESC LIMIT 1
    `, id)
    return r.scanMessage(row)
}

func (r *PostgresRepository) GetMessages(filter *domainChatStorage.MessageFilter) ([]*domainChatStorage.Message, error) {
    base := `SELECT id, chat_jid, sender, content, timestamp, is_from_me, media_type, filename, url, media_key, file_sha256, file_enc_sha256, file_length, created_at, updated_at FROM messages`
    var where []string
    var args []any
    if filter != nil {
        if filter.ChatJID != "" {
            where = append(where, "chat_jid = $"+fmt.Sprint(len(args)+1))
            args = append(args, filter.ChatJID)
        }
        if filter.StartTime != nil {
            where = append(where, "timestamp >= $"+fmt.Sprint(len(args)+1))
            args = append(args, *filter.StartTime)
        }
        if filter.EndTime != nil {
            where = append(where, "timestamp <= $"+fmt.Sprint(len(args)+1))
            args = append(args, *filter.EndTime)
        }
        if filter.MediaOnly {
            where = append(where, "media_type <> ''")
        }
        if filter.IsFromMe != nil {
            where = append(where, "is_from_me = $"+fmt.Sprint(len(args)+1))
            args = append(args, *filter.IsFromMe)
        }
    }
    if len(where) > 0 {
        base += " WHERE " + strings.Join(where, " AND ")
    }
    base += " ORDER BY timestamp DESC"
    if filter != nil && filter.Limit > 0 {
        base += " LIMIT $" + fmt.Sprint(len(args)+1)
        args = append(args, filter.Limit)
    }

    rows, err := r.db.Query(base, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var messages []*domainChatStorage.Message
    for rows.Next() {
        msg, err := r.scanMessage(rows)
        if err != nil {
            return nil, err
        }
        messages = append(messages, msg)
    }
    return messages, rows.Err()
}

func (r *PostgresRepository) SearchMessages(chatJID, searchText string, limit int) ([]*domainChatStorage.Message, error) {
    rows, err := r.db.Query(`
        SELECT id, chat_jid, sender, content, timestamp, is_from_me, media_type, filename, url, media_key, file_sha256, file_enc_sha256, file_length, created_at, updated_at
        FROM messages
        WHERE chat_jid = $1 AND content ILIKE $2
        ORDER BY timestamp DESC
        LIMIT $3
    `, chatJID, "%"+searchText+"%", limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []*domainChatStorage.Message
    for rows.Next() {
        msg, err := r.scanMessage(rows)
        if err != nil {
            return nil, err
        }
        out = append(out, msg)
    }
    return out, rows.Err()
}

func (r *PostgresRepository) DeleteMessage(id, chatJID string) error {
    _, err := r.db.Exec(`DELETE FROM messages WHERE id = $1 AND chat_jid = $2`, id, chatJID)
    return err
}

func (r *PostgresRepository) GetChatMessageCount(chatJID string) (int64, error) {
    return r.getCount(`SELECT COUNT(*) FROM messages WHERE chat_jid = $1`, chatJID)
}

func (r *PostgresRepository) GetTotalMessageCount() (int64, error) {
    return r.getCount(`SELECT COUNT(*) FROM messages`)
}

func (r *PostgresRepository) GetTotalChatCount() (int64, error) {
    return r.getCount(`SELECT COUNT(*) FROM chats`)
}

func (r *PostgresRepository) TruncateAllChats() error {
    _, err := r.db.Exec(`TRUNCATE TABLE messages, chats RESTART IDENTITY CASCADE`)
    return err
}

func (r *PostgresRepository) GetStorageStatistics() (chatCount int64, messageCount int64, err error) {
    chatCount, err = r.GetTotalChatCount()
    if err != nil { return }
    messageCount, err = r.GetTotalMessageCount()
    return
}

func (r *PostgresRepository) TruncateAllDataWithLogging(logPrefix string) error {
    logrus.Infof("%s Truncating chats and messages (Postgres)", logPrefix)
    return r.TruncateAllChats()
}

func (r *PostgresRepository) StoreSentMessageWithContext(ctx context.Context, messageID string, senderJID string, recipientJID string, content string, timestamp time.Time) error {
    msg := &domainChatStorage.Message{
        ID:        messageID,
        ChatJID:   recipientJID,
        Sender:    senderJID,
        Content:   content,
        Timestamp: timestamp,
        IsFromMe:  true,
    }
    return r.StoreMessage(msg)
}

func (r *PostgresRepository) CreateMessage(ctx context.Context, evt *events.Message) error {
    if evt == nil || evt.Message == nil {
        return nil
    }

    chatJID := evt.Info.Chat.String()
    sender := evt.Info.Sender.String()
    chatName := r.GetChatNameWithPushName(evt.Info.Chat, chatJID, evt.Info.Sender.User, evt.Info.PushName)

    // Keep existing ephemeral expiration if any
    existingChat, err := r.GetChat(chatJID)
    if err != nil {
        return fmt.Errorf("get chat failed: %w", err)
    }
    ephemeralExpiration := utils.ExtractEphemeralExpiration(evt.Message)
    chat := &domainChatStorage.Chat{
        JID:             chatJID,
        Name:            chatName,
        LastMessageTime: evt.Info.Timestamp,
    }
    if ephemeralExpiration > 0 {
        chat.EphemeralExpiration = ephemeralExpiration
    } else if existingChat != nil {
        chat.EphemeralExpiration = existingChat.EphemeralExpiration
    }
    if err := r.StoreChat(chat); err != nil {
        return err
    }

    content := utils.ExtractMessageTextFromProto(evt.Message)
    mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength := utils.ExtractMediaInfo(evt.Message)
    if content == "" && mediaType == "" {
        return nil
    }

    return r.StoreMessage(&domainChatStorage.Message{
        ID:            evt.Info.ID,
        ChatJID:       chatJID,
        Sender:        sender,
        Content:       content,
        Timestamp:     evt.Info.Timestamp,
        IsFromMe:      evt.Info.IsFromMe,
        MediaType:     mediaType,
        Filename:      filename,
        URL:           url,
        MediaKey:      mediaKey,
        FileSHA256:    fileSHA256,
        FileEncSHA256: fileEncSHA256,
        FileLength:    fileLength,
    })
}

func (r *PostgresRepository) GetChatNameWithPushName(jid types.JID, chatJID string, senderUser string, pushName string) string {
    // Try existing chat first
    existingChat, err := r.GetChat(chatJID)
    if err == nil && existingChat != nil && existingChat.Name != "" {
        if pushName != "" && (existingChat.Name == jid.User || existingChat.Name == senderUser) {
            return pushName
        }
        return existingChat.Name
    }

    var name string
    switch jid.Server {
    case "g.us":
        name = fmt.Sprintf("Group %s", jid.User)
    case "newsletter":
        name = fmt.Sprintf("Newsletter %s", jid.User)
    default:
        if pushName != "" && pushName != senderUser && pushName != jid.User {
            name = pushName
        } else if senderUser != "" {
            name = senderUser
        } else {
            name = jid.User
        }
    }
    return name
}

func (r *PostgresRepository) InitializeSchema() error {
    version, err := r.getSchemaVersion()
    if err != nil { return err }
    migrations := r.getMigrations()
    for i := version; i < len(migrations); i++ {
        if err := r.runMigration(migrations[i], i+1); err != nil {
            return fmt.Errorf("failed to run migration %d: %w", i+1, err)
        }
    }
    return nil
}

func (r *PostgresRepository) getSchemaVersion() (int, error) {
    _, err := r.db.Exec(`
        CREATE TABLE IF NOT EXISTS schema_info (
            version INTEGER PRIMARY KEY,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    `)
    if err != nil { return 0, err }
    var version int
    if err := r.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_info").Scan(&version); err != nil {
        return 0, err
    }
    return version, nil
}

func (r *PostgresRepository) runMigration(migration string, version int) error {
    tx, err := r.db.Begin()
    if err != nil { return err }
    defer tx.Rollback()
    if _, err := tx.Exec(migration); err != nil { return err }
    if _, err := tx.Exec("INSERT INTO schema_info (version) VALUES ($1) ON CONFLICT (version) DO UPDATE SET updated_at=CURRENT_TIMESTAMP", version); err != nil {
        return err
    }
    return tx.Commit()
}

func (r *PostgresRepository) getMigrations() []string {
    return []string{
        `
        CREATE TABLE IF NOT EXISTS chats (
            jid TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            last_message_time TIMESTAMP NOT NULL,
            ephemeral_expiration INTEGER DEFAULT 0,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
        CREATE TABLE IF NOT EXISTS messages (
            id TEXT NOT NULL,
            chat_jid TEXT NOT NULL,
            sender TEXT NOT NULL,
            content TEXT,
            timestamp TIMESTAMP NOT NULL,
            is_from_me BOOLEAN DEFAULT FALSE,
            media_type TEXT,
            filename TEXT,
            url TEXT,
            media_key BYTEA,
            file_sha256 BYTEA,
            file_enc_sha256 BYTEA,
            file_length INTEGER DEFAULT 0,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (id, chat_jid),
            FOREIGN KEY (chat_jid) REFERENCES chats(jid) ON DELETE CASCADE
        );
        CREATE INDEX IF NOT EXISTS idx_messages_chat_jid ON messages(chat_jid);
        CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp);
        CREATE INDEX IF NOT EXISTS idx_messages_media_type ON messages(media_type);
        CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender);
        CREATE INDEX IF NOT EXISTS idx_chats_last_message ON chats(last_message_time);
        CREATE INDEX IF NOT EXISTS idx_chats_name ON chats(name);
        `,
        `CREATE INDEX IF NOT EXISTS idx_messages_id ON messages(id);`,
        `
        CREATE TABLE IF NOT EXISTS app_users (
            username TEXT PRIMARY KEY,
            password_hash TEXT NOT NULL,
            role TEXT DEFAULT 'admin',
            enabled BOOLEAN DEFAULT TRUE,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );
        CREATE INDEX IF NOT EXISTS idx_app_users_enabled ON app_users(enabled);
        `,
    }
}

func (r *PostgresRepository) getCount(query string, args ...any) (int64, error) {
    var c int64
    err := r.db.QueryRow(query, args...).Scan(&c)
    return c, err
}

func (r *PostgresRepository) scanMessage(scanner interface{ Scan(...any) error }) (*domainChatStorage.Message, error) {
    var m domainChatStorage.Message
    var mediaKey, fileSha, fileEncSha []byte
    err := scanner.Scan(
        &m.ID, &m.ChatJID, &m.Sender, &m.Content, &m.Timestamp, &m.IsFromMe,
        &m.MediaType, &m.Filename, &m.URL, &mediaKey, &fileSha, &fileEncSha, &m.FileLength, &m.CreatedAt, &m.UpdatedAt,
    )
    if err != nil { return nil, err }
    m.MediaKey = mediaKey
    m.FileSHA256 = fileSha
    m.FileEncSHA256 = fileEncSha
    return &m, nil
}

func (r *PostgresRepository) scanChat(scanner interface{ Scan(...any) error }) (*domainChatStorage.Chat, error) {
    var c domainChatStorage.Chat
    err := scanner.Scan(&c.JID, &c.Name, &c.LastMessageTime, &c.EphemeralExpiration, &c.CreatedAt, &c.UpdatedAt)
    if err != nil { return nil, err }
    return &c, nil
}