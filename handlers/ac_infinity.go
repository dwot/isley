package handlers

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"isley/logger"
)

func ACILoginHandler(c *gin.Context) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.WithError(err).Error("Failed to bind JSON request")
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request payload"})
		return
	}

	// Enforce password length limit for AC Infinity API
	if len(req.Password) > 25 {
		req.Password = req.Password[:25]
	}

	// Properly encode form data
	values := url.Values{}
	values.Set("appEmail", req.Email)
	values.Set("appPasswordl", req.Password)
	formData := values.Encode()
	apiURL := "http://www.acinfinityserver.com/api/user/appUserLogin"

	httpRequest, err := http.NewRequest("POST", apiURL, strings.NewReader(formData))
	if err != nil {
		logger.Log.WithError(err).Error("Failed to create HTTP request")
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
		logger.Log.WithError(err).Error("Failed to connect to AC Infinity API")
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to connect to AC Infinity API"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Log.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"status":      resp.Status,
		}).Error("Non-200 response from AC Infinity API")
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
		logger.Log.WithError(err).Error("Failed to decode AC Infinity response")
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to process AC Infinity response"})
		return
	}

	if aciResponse.Code != 200 {
		logger.Log.WithFields(logrus.Fields{
			"code":    aciResponse.Code,
			"message": aciResponse.Msg,
		}).Error("AC Infinity API returned an error")
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": aciResponse.Msg})
		return
	}

	// Update the user's token in the database
	UpdateSetting("aci.token", aciResponse.Data.AppID)
	c.JSON(http.StatusOK, gin.H{"success": true, "token": aciResponse.Data.AppID})
}
