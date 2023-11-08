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
	claimIds := []int{5225127934}
	client := &http.Client{}

	for _, id := range claimIds {
		// GET Request to claims API
		url := fmt.Sprintf("https://internal-api.mercadolibre.com/v1/claims/%d", id)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			fmt.Println("ERROR: " + err.Error())
		}

		req.Header.Add("X-Caller-Scopes", "admin")
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("ERROR: " + err.Error())
		}

		var claimResponse ClaimResponse
		err = json.NewDecoder(resp.Body).Decode(&claimResponse)
		if err != nil {
			fmt.Println("ERROR UNMARSHALL CLAIM")
		}

		// GET Request CX API
		url = fmt.Sprintf("https://internal-api.mercadolibre.com/cx/cases/search/v2?claim_id=%d", id)
		req, err = http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			fmt.Println("ERROR: " + err.Error())
		}

		req.Header.Add("X-Admin-Id", "admin")
		resp, err = client.Do(req)
		if err != nil {
			fmt.Println("ERROR: " + err.Error())
		}

		var cxResponse CXResponse
		err = json.NewDecoder(resp.Body).Decode(&cxResponse)
		if err != nil {
			fmt.Println("ERROR UNMARSHALL CX")
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

		if !hasMandatoryActionExpired {
			msg := fmt.Sprintf("CLAIM %d -> CONSISTENT", id)
			fmt.Println(msg)
			continue
		}

		// Check if the CX case is open
		if len(cxResponse.Results) == 0 {
			msg := fmt.Sprintf("CLAIM %d -> ERROR: ZERO CX CASES", id)
			fmt.Println(msg)
			continue
		}

		if len(cxResponse.Results) > 1 {
			msg := fmt.Sprintf("CLAIM %d -> ERROR: MORE THAN ONE CX CASE", id)
			fmt.Println(msg)
			continue
		}

		if cxResponse.Results[0].Status == "OPENED" {
			msg := fmt.Sprintf("CLAIM %d -> CONSISTENT", id)
			fmt.Println(msg)
			continue
		}

		// Check if the claim is 1.0 or 2.0
		isClaimV1 := true

		url = fmt.Sprintf("https://internal-api.mercadolibre.com/v1/claims/%d/state", id)
		req, err = http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			fmt.Println("ERROR: " + err.Error())
		}

		req.Header.Add("X-Caller-Scopes", "admin")
		resp, err = client.Do(req)
		if err != nil {
			fmt.Println("ERROR: " + err.Error())
		}

		// Execute Deadlines
		if resp.StatusCode == http.StatusNotFound {
			isClaimV1 = true
		} else {
			isClaimV1 = false
		}

		if isClaimV1 {
			url = "https://internal-api.mercadolibre.com/claims/actions/deadline/reprocess"
			body := fmt.Sprintf(`{"claim_ids":[%d]}`, id)

			req, err = http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
			if err != nil {
				fmt.Println("ERROR: " + err.Error())
			}
			req.Header.Add("Content-Type", "application/json")
		} else {
			url = fmt.Sprintf("https://internal-api.mercadolibre.com/post-purchase/state/deadline/process-claim/%d", id)
			req, err = http.NewRequest(http.MethodPost, url, nil)
			if err != nil {
				fmt.Println("ERROR: " + err.Error())
			}
		}

		resp, err = client.Do(req)
		if err != nil {
			fmt.Println("ERROR: " + err.Error())
		}

		var deadlineResponses []DeadlineResponse
		err = json.NewDecoder(resp.Body).Decode(&deadlineResponses)
		if err != nil {
			fmt.Println("ERROR UNMARSHALL DEADLINE RESPONSE")
			continue
		}

		if deadlineResponses[0].AppliedRule == "none" {
			msg := fmt.Sprintf("CLAIM %d -> REPORT IN THE CORE-CX CHANNEL", id)
			fmt.Println(msg)
		} else {
			msg := fmt.Sprintf("CLAIM %d -> CONSISTENT", id)
			fmt.Println(msg)
		}
	}
}
