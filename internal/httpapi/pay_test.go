package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rupeshkoushik07/lightning-payments-service/internal/store"
)

type fakePause struct {
	paused bool
	err    error
}

func (p fakePause) PayoutsPaused(context.Context) (bool, error) { return p.paused, p.err }

type fakeDecoder struct {
	inv Invoice
	err error
}

func (d fakeDecoder) Decode(string) (Invoice, error) { return d.inv, d.err }

type fakeSaver struct {
	err   error
	saved int
}

func (s *fakeSaver) CreatePayment(context.Context, store.NewPayment) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	s.saved++
	return 1, nil
}

func TestPayHandler(t *testing.T) {
	good := Invoice{PaymentHash: make([]byte, 32), AmountSat: 1000}

	cases := []struct {
		name      string
		pause     fakePause
		decoder   fakeDecoder
		saveErr   error
		body      string
		wantCode  int
		wantSaved int
	}{
		{"accepted", fakePause{}, fakeDecoder{inv: good}, nil, `{"invoice":"x"}`, http.StatusAccepted, 1},
		{"payouts paused", fakePause{paused: true}, fakeDecoder{inv: good}, nil, `{"invoice":"x"}`, http.StatusServiceUnavailable, 0},
		{"pause switch unreadable: refuse", fakePause{err: errors.New("db down")}, fakeDecoder{inv: good}, nil, `{"invoice":"x"}`, http.StatusServiceUnavailable, 0},
		{"body is not JSON", fakePause{}, fakeDecoder{inv: good}, nil, `hello`, http.StatusBadRequest, 0},
		{"not an invoice", fakePause{}, fakeDecoder{err: errors.New("bad")}, nil, `{"invoice":"x"}`, http.StatusBadRequest, 0},
		{"expired invoice", fakePause{}, fakeDecoder{inv: Invoice{PaymentHash: good.PaymentHash, AmountSat: 1000, Expired: true}}, nil, `{"invoice":"x"}`, http.StatusUnprocessableEntity, 0},
		{"over the limit", fakePause{}, fakeDecoder{inv: Invoice{PaymentHash: good.PaymentHash, AmountSat: 150_000}}, nil, `{"invoice":"x"}`, http.StatusUnprocessableEntity, 0},
		{"no amount in invoice", fakePause{}, fakeDecoder{inv: Invoice{PaymentHash: good.PaymentHash}}, nil, `{"invoice":"x"}`, http.StatusUnprocessableEntity, 0},
		{"already being paid", fakePause{}, fakeDecoder{inv: good}, store.ErrAlreadyPaying, `{"invoice":"x"}`, http.StatusConflict, 0},
		{"database error", fakePause{}, fakeDecoder{inv: good}, errors.New("db down"), `{"invoice":"x"}`, http.StatusInternalServerError, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			saver := &fakeSaver{err: c.saveErr}
			h := &PayHandler{Pause: c.pause, Decoder: c.decoder, Payments: saver, MaxAmountSat: 100_000}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/payments", strings.NewReader(c.body)))
			if rec.Code != c.wantCode || saver.saved != c.wantSaved {
				t.Errorf("code=%d saved=%d, want code=%d saved=%d", rec.Code, saver.saved, c.wantCode, c.wantSaved)
			}
		})
	}
}
