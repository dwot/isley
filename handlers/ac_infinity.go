package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ACILoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func ACILoginHandler(c *gin.Context) {
	var req ACILoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Println("Error parsing request:", err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request payload"})
		return
	}

	// Prepare the payload for the AC Infinity API
	formData := "appEmail=" + req.Email + "&appPasswordl=" + req.Password
	apiURL := "http://www.acinfinityserver.com/api/user/appUserLogin"

	// Create a new HTTP request
	httpRequest, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(formData))
	if err != nil {
		log.Println("Error creating HTTP request:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to create request"})
		return
	}

	// Set headers
	httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	httpRequest.Header.Set("User-Agent", "ACController/1.8.2 (com.acinfinity.humiture; build:489; iOS 16.5.1) Alamofire/5.4.4")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(httpRequest)
	if err != nil {
		log.Println("Error calling AC Infinity API:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to connect to AC Infinity API"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("Non-200 response from AC Infinity API:", resp.Status)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch token"})
		return
	}

	var aciResponse struct {
		Msg  string `json:"msg"`
		Code int    `json:"code"`
		Data struct {
			AppID string `json:"appId"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&aciResponse); err != nil {
		log.Println("Error decoding AC Infinity response:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to process AC Infinity response"})
		return
	}

	if aciResponse.Code != 200 {
		log.Println("AC Infinity API error:", aciResponse.Msg)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": aciResponse.Msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "token": aciResponse.Data.AppID})
}
