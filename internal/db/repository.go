package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	cryptostore "polycom-manager/internal/crypto"
	"polycom-manager/internal/device"
)

type Repository struct {
	db     *sql.DB
	crypto *cryptostore.Store
}

func NewRepository(db *sql.DB, crypto *cryptostore.Store) *Repository {
	return &Repository{db: db, crypto: crypto}
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Repository) ListDevices(ctx context.Context) ([]device.Device, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,host,port,model,location,username,credential_id,enabled,created_at,updated_at FROM devices ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []device.Device
	for rows.Next() {
		var d device.Device
		var enabled int
		var created, updated string
		if err := rows.Scan(&d.ID, &d.Name, &d.Host, &d.Port, &d.Model, &d.Location, &d.Username, &d.CredentialID, &enabled, &created, &updated); err != nil {
			return nil, err
		}
		d.Enabled = enabled != 0
		d.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *Repository) GetDevice(ctx context.Context, id string) (device.Device, error) {
	var d device.Device
	var enabled int
	var created, updated string
	err := r.db.QueryRowContext(ctx, `SELECT id,name,host,port,model,location,username,credential_id,enabled,created_at,updated_at FROM devices WHERE id=?`, id).
		Scan(&d.ID, &d.Name, &d.Host, &d.Port, &d.Model, &d.Location, &d.Username, &d.CredentialID, &enabled, &created, &updated)
	if err != nil {
		return d, err
	}
	d.Enabled = enabled != 0
	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return d, nil
}

func (r *Repository) GetPassword(ctx context.Context, credentialID string) (string, error) {
	var ciphertext, nonce []byte
	if err := r.db.QueryRowContext(ctx, `SELECT ciphertext,nonce FROM credentials WHERE id=?`, credentialID).Scan(&ciphertext, &nonce); err != nil {
		return "", err
	}
	return r.crypto.Decrypt(ciphertext, nonce)
}

func (r *Repository) CreateDevice(ctx context.Context, in device.UpsertInput) (device.Device, error) {
	if err := in.Validate(true); err != nil {
		return device.Device{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id, credID := randomID(), randomID()
	ct, nonce, err := r.crypto.Encrypt(in.Password)
	if err != nil {
		return device.Device{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return device.Device{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO credentials(id,ciphertext,nonce,created_at,updated_at) VALUES(?,?,?,?,?)`, credID, ct, nonce, now, now); err != nil {
		return device.Device{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO devices(id,name,host,port,model,location,username,credential_id,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, in.Name, in.Host, in.Port, in.Model, in.Location, in.Username, credID, boolInt(in.Enabled), now, now); err != nil {
		return device.Device{}, err
	}
	if err = tx.Commit(); err != nil {
		return device.Device{}, err
	}
	return r.GetDevice(ctx, id)
}

func (r *Repository) UpdateDevice(ctx context.Context, id string, in device.UpsertInput) (device.Device, error) {
	if err := in.Validate(false); err != nil {
		return device.Device{}, err
	}
	d, err := r.GetDevice(ctx, id)
	if err != nil {
		return device.Device{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return device.Device{}, err
	}
	defer tx.Rollback()
	if in.Password != "" {
		ct, nonce, err := r.crypto.Encrypt(in.Password)
		if err != nil {
			return device.Device{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE credentials SET ciphertext=?,nonce=?,updated_at=? WHERE id=?`, ct, nonce, now, d.CredentialID); err != nil {
			return device.Device{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE devices SET name=?,host=?,port=?,model=?,location=?,username=?,enabled=?,updated_at=? WHERE id=?`, in.Name, in.Host, in.Port, in.Model, in.Location, in.Username, boolInt(in.Enabled), now, id); err != nil {
		return device.Device{}, err
	}
	if err = tx.Commit(); err != nil {
		return device.Device{}, err
	}
	return r.GetDevice(ctx, id)
}

func (r *Repository) DeleteDevice(ctx context.Context, id string) error {
	d, err := r.GetDevice(ctx, id)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM devices WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM credentials WHERE id=?`, d.CredentialID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) AddAudit(ctx context.Context, deviceID, operation, command, result, detail string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO audit_logs(device_id,operation,command,result,detail,created_at) VALUES(?,?,?,?,?,?)`, nullable(deviceID), operation, command, result, detail, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (r *Repository) ListAudit(ctx context.Context, limit int) ([]device.AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,COALESCE(device_id,''),operation,command,result,detail,created_at FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []device.AuditEntry
	for rows.Next() {
		var a device.AuditEntry
		var ts string
		if err := rows.Scan(&a.ID, &a.DeviceID, &a.Operation, &a.Command, &a.Result, &a.Detail, &ts); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, a)
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func IsNotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }

func (r *Repository) DebugCount(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count devices: %w", err)
	}
	return n, nil
}
