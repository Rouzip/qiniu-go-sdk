package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/qiniupd/qiniu-go-sdk/api.v8/auth/qbox"
	"github.com/qiniupd/qiniu-go-sdk/gas/config"
	"github.com/qiniupd/qiniu-go-sdk/gas/logger"
)

// Client is the api client for Gas APIS
type Client struct {
	config *config.Config
	client *http.Client
	logger logger.Logger
}

// NewClient creates Gas Client
func NewClient(config *config.Config) *Client {
	mac := &qbox.Mac{
		AccessKey: config.AccessKey,
		SecretKey: []byte(config.SecretKey),
	}
	client := qbox.NewClient(mac, http.DefaultTransport)
	// 如果后边接口走 long polling 这个 timeout 需要干掉
	client.Timeout = 10 * time.Second
	return &Client{
		config: config,
		client: client,
	}
}

// SetLogger 设置 logger
func (c *Client) SetLogger(logger logger.Logger) {
	c.logger = logger
}

// RespBody 是所有接口响应 body 的标准格式
type RespBody struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func (c *Client) request(method, path string, reqData interface{}, respData interface{}) (err error) {
	jsonText, err := json.Marshal(reqData)
	if err != nil {
		c.logger.Error("json.Marshal() failed: ", err)
		return
	}

	apiPrefix := c.config.APIPrefix
	if apiPrefix == "" {
		apiPrefix = config.DefaultAPIPrefix
	}

	req, err := http.NewRequest(method, apiPrefix+path, bytes.NewReader(jsonText))
	if err != nil {
		c.logger.Error("http.NewRequest() failed: ", err)
		return
	}

	c.logger.Debug(req.Method + " " + req.URL.String() + " " + string(jsonText))

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Error("c.client.Do() failed: ", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("ioutil.ReadAll() failed: ", err)
		return
	}

	c.logger.Debug(resp.Status + " " + string(bodyBytes))

	if resp.StatusCode != 200 {
		err = fmt.Errorf("Status not ok: %d", resp.StatusCode)
		c.logger.Error("check resp.StatusCode failed: ", err)
		return
	}

	respBody := &RespBody{}
	if respData != nil {
		respBody.Data = respData
	}

	err = json.Unmarshal(bodyBytes, respBody)
	if err != nil {
		c.logger.Error("json.Unmarshal() failed: ", err)
		return
	}

	err = Ensure(respBody.Code, respBody.Message)
	return
}

type updateActionReqBody struct {
	MinerID      string `json:"mid"`
	SealingID    string `json:"sno"`
	ActionName   string `json:"act"`
	ActionStatus string `json:"actStat"` // Start / End
}

// UpdateAction 调用接口更新 action 状态
func (c *Client) UpdateAction(sealingID, action string, actionStatus string) error {
	reqBody := &updateActionReqBody{
		MinerID:      c.config.MinerID,
		SealingID:    sealingID,
		ActionName:   action,
		ActionStatus: actionStatus,
	}

	return c.request("POST", "/v1/sector/action", reqBody, nil)
}

// SealingData 是 sealing 条目的内容
type SealingData struct {
	MinerID        string `json:"Miner_ID"`
	SealingID      string `json:"Sealing_ID"`
	PSSStartTime   int64  `json:"PSS_StartTime"`
	PSSWWaitTime   int64  `json:"PSSW_WaitTime"`
	PreCSStartTime int64  `json:"PreCS_StartTime"`
	WSTime         int64  `json:"WS_Time"`
	CStartTime     int64  `json:"C_StartTime"`
	CWTime         int64  `json:"CW_Time"`
	ProCSStartTime int64  `json:"ProCS_StartTime"`
	ProCSEndTime   int64  `json:"ProCS_EndTime"`
	SealingStatus  int    `json:"Sealing_Status"`
	CancelTime     int64  `json:"Cancel_Time"`
	CreateTime     int64  `json:"Create_Time"`
}

// GetSealing 获取 sealingID 对应的条目信息
func (c *Client) GetSealing(sealingID string) (*SealingData, error) {
	path := fmt.Sprintf("/v1/sector/%s/%s", c.config.MinerID, sealingID)
	data := &SealingData{}
	err := c.request("GET", path, nil, data)
	return data, err
}

type cancelReqBody struct {
	MinerID   string `json:"mid"`
	SealingID string `json:"sno"`
}

// CancelSealing 标记取消 sealingID 对应
func (c *Client) CancelSealing(sealingID string) error {
	reqBody := &cancelReqBody{
		MinerID:   c.config.MinerID,
		SealingID: sealingID,
	}
	return c.request("POST", "/v1/sector/cancel", reqBody, nil)
}

// CheckActionData 是检查 Action 是否可执行的返回结果
type CheckActionData struct {
	Ok   bool `json:"ok"`
	Wait uint `json:"wait"`
}

// CheckAction 检查 Action 是否可执行
func (c *Client) CheckAction(sealingID, action string, t *int64) (*CheckActionData, error) {
	path := fmt.Sprintf("/v1/sector/check/%s/%s/%s", c.config.MinerID, sealingID, action)
	if t != nil {
		path = path + "?time=" + strconv.FormatInt(*t, 10)
	}
	data := &CheckActionData{}
	err := c.request("GET", path, nil, data)
	return data, err
}

// UserConfig 是用户级别的配置信息
type UserConfig struct {
	MaxBaseFee       float32  `json:"mbf"`
	MaxPreCommitMsg  int      `json:"mpm"`
	SealNumberPer24h int      `json:"snp"`
	MaxIntervalTime  int      `json:"mit"`
	MinerIDs         []string `json:"mids"`
}

// GetUserConfig 用来获取用户级别的配置信息
func (c *Client) GetUserConfig() (*UserConfig, error) {
	data := &UserConfig{}
	err := c.request("GET", "/v1/gas/config", nil, data)
	return data, err
}

// SetUserConfig 用来设置用户级别的配置信息
func (c *Client) SetUserConfig(userConfig *UserConfig) error {
	return c.request("POST", "/v1/gas/config", userConfig, nil)
}
