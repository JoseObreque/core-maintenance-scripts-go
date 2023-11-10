package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ClaimResponse struct {
	Players []Player `json:"players"`
}

type Player struct {
	Role             string   `json:"role"`
	AvailableActions []Action `json:"available_actions"`
}

type Action struct {
	DueDate   time.Time `json:"due_date"`
	Mandatory bool      `json:"mandatory"`
}

type CXResponse struct {
	Results []CXResult `json:"results"`
}

type CXResult struct {
	Status string `json:"status"`
}

type DeadlineResponse struct {
	AppliedRule string `json:"applied_rule"`
}

func main() {
	claimIds := []int{
		000000000,
	}

	client := &http.Client{}

	for _, id := range claimIds {
		// GET Request to claims API
		url := fmt.Sprintf("https://internal-api.mercadolibre.com/v1/claims/%d", id)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			msg := fmt.Sprintf("%d -> ERROR: CANNOT CREATE GET REQUEST", id)
			fmt.Println(msg)
			continue
		}

		req.Header.Add("X-Caller-Scopes", "admin")
		resp, err := client.Do(req)
		if err != nil {
			msg := fmt.Sprintf("%d -> ERROR: CANNOT EXECUTE GET REQUEST", id)
			fmt.Println(msg)
			continue
		}

		var claimResponse ClaimResponse
		err = json.NewDecoder(resp.Body).Decode(&claimResponse)
		if err != nil {
			msg := fmt.Sprintf("%d -> ERROR: CANNOT UNMARSHALL RESPONSE", id)
			fmt.Println(msg)
			continue
		}

		// Check if there is any mandatory action expired for mediator player in the claim response
		hasMandatoryActionExpired := false
		for _, player := range claimResponse.Players {
			if player.Role == "mediator" {
				for _, action := range player.AvailableActions {
					if action.Mandatory && action.DueDate.Before(time.Now()) {
						hasMandatoryActionExpired = true
						break
					}
				}
			}
		}

		// If there is no mandatory action expired, the claim is already consistent
		if !hasMandatoryActionExpired {
			msg := fmt.Sprintf("%d -> CONSISTENTE", id)
			fmt.Println(msg)
			continue
		}

		// GET Request CX API
		url = fmt.Sprintf("https://internal-api.mercadolibre.com/cx/cases/search/v2?claim_id=%d", id)
		req, err = http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			msg := fmt.Sprintf("%d -> ERROR: CANNOT CREATE GET CX REQUEST", id)
			fmt.Println(msg)
			continue
		}

		req.Header.Add("X-Admin-Id", "admin")
		resp, err = client.Do(req)
		if err != nil {
			msg := fmt.Sprintf("%d -> ERROR: CANNOT EXECUTE GET CX REQUEST", id)
			fmt.Println(msg)
			continue
		}

		var cxResponse CXResponse
		err = json.NewDecoder(resp.Body).Decode(&cxResponse)
		if err != nil {
			msg := fmt.Sprintf("%d -> ERROR: CANNOT UNMARSHALL CX RESPONSE", id)
			fmt.Println(msg)
			continue
		}

		// Check if the CX case is open
		if len(cxResponse.Results) == 0 {
			msg := fmt.Sprintf("%d -> SIN CASO EN CX", id)
			fmt.Println(msg)
			continue
		}

		if len(cxResponse.Results) > 1 {
			msg := fmt.Sprintf("%d -> MAS DE UN CASO EN CX", id)
			fmt.Println(msg)
			continue
		}

		cxStatus := cxResponse.Results[0].Status

		if cxStatus == "OPENED" {
			msg := fmt.Sprintf("%d -> CONSISTENTE", id)
			fmt.Println(msg)
			continue
		}

		// If the CX status is not OPENED or CLOSED, there is no actual action to take
		if cxStatus != "CLOSED" {
			msg := fmt.Sprintf("%d -> CX STATUS: %s", id, cxStatus)
			fmt.Println(msg)
			continue
		}

		// Check if the claim is 1.0 or 2.0
		isClaimV1 := true

		url = fmt.Sprintf("https://internal-api.mercadolibre.com/v1/claims/%d/state", id)
		req, err = http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			msg := fmt.Sprintf("%d -> ERROR: CANNOT CREATE GET STATE REQUEST", id)
			fmt.Println(msg)
			continue
		}

		req.Header.Add("X-Caller-Scopes", "admin")
		resp, err = client.Do(req)
		if err != nil {
			msg := fmt.Sprintf("%d -> ERROR: CANNOT EXECUTE GET STATE REQUEST", id)
			fmt.Println(msg)
			continue
		}

		// Execute Deadlines
		if resp.StatusCode == http.StatusOK {
			isClaimV1 = false
		} else if resp.StatusCode == http.StatusNotFound {
			isClaimV1 = true
		} else {
			msg := fmt.Sprintf("%d -> ERROR: HTTP %d IN STATE REQUEST", id, resp.StatusCode)
			fmt.Println(msg)
			continue
		}

		if isClaimV1 {
			url = "https://internal-api.mercadolibre.com/claims/actions/deadline/reprocess"
			body := fmt.Sprintf(`{"claim_ids":[%d]}`, id)

			req, err = http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
			if err != nil {
				fmt.Println("ERROR: " + err.Error())
				continue
			}
			req.Header.Add("Content-Type", "application/json")
		} else {
			url = fmt.Sprintf("https://internal-api.mercadolibre.com/post-purchase/state/deadline/process-claim/%d", id)
			req, err = http.NewRequest(http.MethodPost, url, nil)
			if err != nil {
				fmt.Println("ERROR: " + err.Error())
				continue
			}
		}

		resp, err = client.Do(req)
		if err != nil {
			fmt.Println("ERROR: " + err.Error())
			continue
		}

		var deadlineResponses []DeadlineResponse
		err = json.NewDecoder(resp.Body).Decode(&deadlineResponses)
		if err != nil {
			fmt.Println("ERROR UNMARSHALL DEADLINE RESPONSE")
			continue
		}

		if deadlineResponses[0].AppliedRule == "none" {
			msg := fmt.Sprintf("%d -> SE SOLICITA REAPERTURA EN CX", id)
			fmt.Println(msg)
		} else {
			msg := fmt.Sprintf("%d -> CONSISTENTE", id)
			fmt.Println(msg)
		}
	}
}
