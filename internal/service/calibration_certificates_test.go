package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

// fixedAuditIDGen returns a fixed ID on every call. Used to force the audit
// INSERT in CalibrationCertificateService.Create to collide on the
// audit_records primary key so the audit step fails mid-transaction.
type fixedAuditIDGen struct{ id string }

func (g fixedAuditIDGen) New() string { return g.id }

func newCalibrationFixture(t *testing.T) (*store.Store, *CalibrationCertificateService, *slog.Logger) {
	t.Helper()
	st, _ := storeTestStore(t)
	clock := platform.SystemClock{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, NewCalibrationCertificateService(st, clock, platform.RandomIDGenerator{}, log), log
}

func newCalibrationCertificate(id string, now time.Time, no string) *domain.CalibrationCertificate {
	c := domain.NewCalibrationCertificate(id, now)
	c.EquipmentID = "eq-1"
	c.CertificateNo = no
	c.IssuedAt = now
	c.ExpiresAt = now.Add(24 * time.Hour)
	return c
}

// TestCalibrationCertificateCreateWritesAudit verifies the requested invariant:
// a normal create must commit both the certificate row and its audit record
// inside a single transaction, so a successful create always leaves an audit.
func TestCalibrationCertificateCreateWritesAudit(t *testing.T) {
	ctx := context.Background()
	st, svc, _ := newCalibrationFixture(t)
	defer st.Close()

	now := time.Now().UTC()
	cert := newCalibrationCertificate("cert-ok", now, "C-001")
	if _, err := svc.Create(ctx, cert, ""); err != nil {
		t.Fatalf("create: %v", err)
	}

	// The certificate row must be readable.
	got, err := st.CalibrationCertificate.Get(ctx, st.DB, "cert-ok")
	if err != nil {
		t.Fatalf("get certificate: %v", err)
	}
	if got.CertificateNo != "C-001" {
		t.Fatalf("unexpected certificate_no %s", got.CertificateNo)
	}

	// Exactly one "create" audit row must exist for this certificate.
	audits, total, err := st.AuditRecord.List(ctx, st.DB, map[string]any{
		"entity":    "calibrationcertificate",
		"entity_id": "cert-ok",
		"action":    "create",
	}, domain.Page{Page: 1, Size: 10}, "")
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if total != 1 || len(audits) != 1 {
		t.Fatalf("expected exactly one create audit for cert-ok, got total=%d len=%d", total, len(audits))
	}
	if audits[0].EntityID != "cert-ok" {
		t.Fatalf("audit entity_id mismatch: %s", audits[0].EntityID)
	}
}

// TestCalibrationCertificateCreateAtomicOnAuditFailure verifies the fix for the
// reported orphan: if the audit write fails, the whole create must roll back so
// no certificate row is left behind. Before the fix the certificate was written
// outside the transaction (autocommit), so an audit failure left an orphan whose
// meta even carried write_scope="autocommit".
func TestCalibrationCertificateCreateAtomicOnAuditFailure(t *testing.T) {
	ctx := context.Background()
	st, _, log := newCalibrationFixture(t)
	defer st.Close()

	now := time.Now().UTC()

	// Pre-seed an audit row whose ID the service will try to reuse, so the audit
	// INSERT inside Create collides on the primary key and fails.
	const collidingAuditID = "audit-collide"
	seed := domain.NewAuditRecord(collidingAuditID, now)
	seed.Entity = "calibrationcertificate"
	seed.EntityID = "preseed"
	seed.Action = "create"
	seed.Actor = "system"
	if err := st.AuditRecord.Create(ctx, st.DB, seed); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	clock := platform.SystemClock{}
	svcFail := NewCalibrationCertificateService(st, clock, fixedAuditIDGen{id: collidingAuditID}, log)
	cert := newCalibrationCertificate("cert-fail", now, "C-002")

	if _, err := svcFail.Create(ctx, cert, ""); err == nil {
		t.Fatalf("expected audit failure to surface as an error")
	}

	// The certificate must NOT be readable: the create transaction rolled back
	// together with the failed audit, leaving no orphan.
	_, err := st.CalibrationCertificate.Get(ctx, st.DB, "cert-fail")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected orphan certificate to be absent after audit failure, got err=%v", err)
	}

	// The only audit row for cert-fail's id is the pre-seed; no audit recording a
	// successful create of cert-fail should exist.
	audits, total, err := st.AuditRecord.List(ctx, st.DB, map[string]any{
		"entity_id": "cert-fail",
	}, domain.Page{Page: 1, Size: 10}, "")
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if total != 0 || len(audits) != 0 {
		t.Fatalf("expected no audit for cert-fail, got total=%d len=%d", total, len(audits))
	}
}
