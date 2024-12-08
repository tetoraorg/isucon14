package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/bytedance/sonic/decoder"
	"github.com/bytedance/sonic/encoder"
)

var erroredUpstream = errors.New("errored upstream")

type paymentGatewayPostPaymentRequest struct {
	Amount int `json:"amount"`
}

type paymentGatewayGetPaymentsResponseOne struct {
	Amount int    `json:"amount"`
	Status string `json:"status"`
}

func requestPaymentGatewayPostPayment(ctx context.Context, paymentGatewayURL string, token string, param *paymentGatewayPostPaymentRequest, retrieveRidesOrderByCreatedAtAsc func() ([]Ride, error)) error {
	buf := new(bytes.Buffer)
	if err := encoder.NewStreamEncoder(buf).Encode(param); err != nil {
		return fmt.Errorf("failed to encode param: %w", err)
	}

	const maxRetries = 5
	const retryDelay = 100 * time.Millisecond

	for retry := 0; retry < maxRetries; retry++ {
		if err := tryPostAndValidate(ctx, paymentGatewayURL, token, buf.Bytes(), retrieveRidesOrderByCreatedAtAsc); err != nil {
			// リトライ条件判定（ここでは単純に回数制限のみ）
			if retry < maxRetries-1 {
				time.Sleep(retryDelay)
				continue
			}
			return fmt.Errorf("all retries failed: %w", err)
		}
		return nil
	}

	return nil
}

func tryPostAndValidate(ctx context.Context, paymentGatewayURL, token string, body []byte, retrieveRidesOrderByCreatedAtAsc func() ([]Ride, error)) error {
	// POST /payments
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, paymentGatewayURL+"/payments", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create POST request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST /payments request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNoContent {
		// 成功
		return nil
	}

	// POSTが204以外の場合はGET /paymentsで確認
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, paymentGatewayURL+"/payments", nil)
	if err != nil {
		return fmt.Errorf("create GET request failed: %w", err)
	}
	getReq.Header.Set("Authorization", "Bearer "+token)

	getRes, err := http.DefaultClient.Do(getReq)
	if err != nil {
		return fmt.Errorf("GET /payments request failed: %w", err)
	}
	defer getRes.Body.Close()

	if getRes.StatusCode != http.StatusOK {
		return fmt.Errorf("[GET /payments] unexpected status code (%d)", getRes.StatusCode)
	}

	var payments []paymentGatewayGetPaymentsResponseOne
	if err := decoder.NewStreamDecoder(getRes.Body).Decode(&payments); err != nil {
		return fmt.Errorf("decode /payments response failed: %w", err)
	}

	rides, err := retrieveRidesOrderByCreatedAtAsc()
	if err != nil {
		return fmt.Errorf("retrieveRidesOrderByCreatedAtAsc failed: %w", err)
	}

	if len(rides) != len(payments) {
		return fmt.Errorf("unexpected number of payments: %d != %d. %w", len(rides), len(payments), erroredUpstream)
	}

	// POSTは204でなくてもここまでくれば整合性が取れていると見なす
	return nil
}

// func requestPaymentGatewayPostPayment(ctx context.Context, paymentGatewayURL string, token string, param *paymentGatewayPostPaymentRequest, retrieveRidesOrderByCreatedAtAsc func() ([]Ride, error)) error {
// 	buf := new(bytes.Buffer)
// 	err := encoder.NewStreamEncoder(buf).Encode(param)
// 	if err != nil {
// 		return err
// 	}

// 	// 失敗したらとりあえずリトライ
// 	// FIXME: 社内決済マイクロサービスのインフラに異常が発生していて、同時にたくさんリクエストすると変なことになる可能性あり
// 	retry := 0
// 	for {
// 		err := func() error {
// 			req, err := http.NewRequestWithContext(ctx, http.MethodPost, paymentGatewayURL+"/payments", bytes.NewBuffer(buf.Bytes()))
// 			if err != nil {
// 				return err
// 			}
// 			req.Header.Set("Content-Type", "application/json")
// 			req.Header.Set("Authorization", "Bearer "+token)

// 			res, err := http.DefaultClient.Do(req)
// 			if err != nil {
// 				return err
// 			}
// 			defer res.Body.Close()

// 			if res.StatusCode != http.StatusNoContent {
// 				// エラーが返ってきても成功している場合があるので、社内決済マイクロサービスに問い合わせ
// 				getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, paymentGatewayURL+"/payments", bytes.NewBuffer([]byte{}))
// 				if err != nil {
// 					return err
// 				}
// 				getReq.Header.Set("Authorization", "Bearer "+token)

// 				getRes, err := http.DefaultClient.Do(getReq)
// 				if err != nil {
// 					return err
// 				}
// 				defer res.Body.Close()

// 				// GET /payments は障害と関係なく200が返るので、200以外は回復不能なエラーとする
// 				if getRes.StatusCode != http.StatusOK {
// 					return fmt.Errorf("[GET /payments] unexpected status code (%d)", getRes.StatusCode)
// 				}
// 				var payments []paymentGatewayGetPaymentsResponseOne
// 				if err := decoder.NewStreamDecoder(getRes.Body).Decode(&payments); err != nil {
// 					return err
// 				}

// 				rides, err := retrieveRidesOrderByCreatedAtAsc()
// 				if err != nil {
// 					return err
// 				}

// 				if len(rides) != len(payments) {
// 					return fmt.Errorf("unexpected number of payments: %d != %d. %w", len(rides), len(payments), erroredUpstream)
// 				}

// 				return nil
// 			}
// 			return nil
// 		}()
// 		if err != nil {
// 			if retry < 5 {
// 				retry++
// 				time.Sleep(100 * time.Millisecond)
// 				continue
// 			} else {
// 				return err
// 			}
// 		}
// 		break
// 	}

// 	return nil
// }
