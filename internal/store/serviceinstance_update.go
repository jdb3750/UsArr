package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ServiceInstanceUpdate carries the mutable fields of a service instance.
//
// Every field is a pointer so that "not supplied" and "supplied as the zero
// value" are different things: a PATCH that omits `enabled` must not disable the
// instance, and a PATCH that sends `"priority": 0` must set it to zero.
type ServiceInstanceUpdate struct {
	Name       *string
	BaseURL    *string
	URLBase    *string
	APIVersion *string
	Enabled    *bool
	Priority   *int64
}

// UpdateServiceInstance applies field changes and, optionally, a new credential
// envelope — in ONE transaction.
//
// reseal exists because of the AAD. The envelope is bound to the row's primary
// key AND to the instance's normalised host:port
// (docs/reference/security.md §1.2), so changing base_url makes the stored
// ciphertext fail to open. The re-seal therefore has to be part of the same
// transaction as the base_url write: committing one without the other leaves a
// row whose credential can never be opened again, with no record of which half
// happened. reseal is called with the row id after the field update and before
// commit; nil means the credential is untouched.
func (s *Store) UpdateServiceInstance(
	ctx context.Context,
	id int64,
	u ServiceInstanceUpdate,
	reseal func(id int64) (envelope []byte, kekID uint32, err error),
) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var sets []string
		var args []any
		if u.Name != nil {
			sets, args = append(sets, "name = ?"), append(args, *u.Name)
		}
		if u.BaseURL != nil {
			sets, args = append(sets, "base_url = ?"), append(args, *u.BaseURL)
		}
		if u.URLBase != nil {
			sets, args = append(sets, "url_base = ?"), append(args, *u.URLBase)
		}
		if u.APIVersion != nil {
			sets, args = append(sets, "api_version = ?"), append(args, nullString(*u.APIVersion))
		}
		if u.Enabled != nil {
			sets, args = append(sets, "enabled = ?"), append(args, *u.Enabled)
		}
		if u.Priority != nil {
			sets, args = append(sets, "priority = ?"), append(args, *u.Priority)
		}

		if len(sets) > 0 {
			args = append(args, id)
			// #nosec G202 -- every element of sets is a string literal from the
			// block above; the VALUES are bound parameters. SQLite has no way to
			// bind a column name, so a variable SET clause has to be assembled.
			res, err := tx.ExecContext(ctx,
				`UPDATE service_instance SET `+strings.Join(sets, ", ")+
					` WHERE id = ? AND deleted_at IS NULL`, args...)
			if err != nil {
				return fmt.Errorf("update service_instance %d: %w", id, err)
			}
			if err := expectOneRow(res, "service_instance", id); err != nil {
				return err
			}
		}

		if reseal == nil {
			return nil
		}
		envelope, kekID, err := reseal(id)
		if err != nil {
			return fmt.Errorf("re-seal api key for service_instance %d: %w", id, err)
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE service_instance SET api_key_enc = ?, kek_id = ? WHERE id = ? AND deleted_at IS NULL`,
			envelope, kekID, id)
		if err != nil {
			return fmt.Errorf("update service_instance %d credential: %w", id, err)
		}
		return expectOneRow(res, "service_instance", id)
	})
}
