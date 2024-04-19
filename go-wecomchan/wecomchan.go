package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"io"
	"io/ioutil"
	"log"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"
)

/*-------------------------------  环境变量配置 begin  -------------------------------*/

var Sendkey = GetEnvDefault("SENDKEY", "set_a_sendkey")
var WecomCid = GetEnvDefault("WECOM_CID", "企业微信公司ID")
var WecomSecret = GetEnvDefault("WECOM_SECRET", "企业微信应用Secret")
var WecomAid = GetEnvDefault("WECOM_AID", "企业微信应用ID")
var WecomToUid = GetEnvDefault("WECOM_TOUID", "@all")
var RedisStat = GetEnvDefault("REDIS_STAT", "OFF")
var RedisAddr = GetEnvDefault("REDIS_ADDR", "localhost:6379")
var RedisPassword = GetEnvDefault("REDIS_PASSWORD", "")
var WecomToken = GetEnvDefault("WECOM_TOKEN", "企业微信回调Token")
var WecomAesKey = GetEnvDefault("WECOM_AES_KEY", "企业微信回调AesKey")
var ctx = context.Background()

/*-------------------------------  环境变量配置 end  -------------------------------*/

/*-------------------------------  企业微信服务端API begin  -------------------------------*/

var GetTokenApi = "https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s"
var SendMessageApi = "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s"
var UploadMediaApi = "https://qyapi.weixin.qq.com/cgi-bin/media/upload?access_token=%s&type=%s"

/*-------------------------------  企业微信服务端API end  -------------------------------*/

const RedisTokenKey = "access_token"

type Msg struct {
	Content string `json:"content"`
}
type Markdown struct {
	Content string `json:"content"`
}
type Pic struct {
	MediaId string `json:"media_id"`
}
type JsonData struct {
	ToUser                 string   `json:"touser"`
	AgentId                string   `json:"agentid"`
	MsgType                string   `json:"msgtype"`
	DuplicateCheckInterval int      `json:"duplicate_check_interval"`
	Text                   Msg      `json:"text"`
	Image                  Pic      `json:"image"`
	Markdown               Markdown `json:"markdown"`
}

// GetEnvDefault 获取配置信息，未获取到则取默认值
func GetEnvDefault(key, defVal string) string {
	val, ex := os.LookupEnv(key)
	if !ex {
		return defVal
	}
	return val
}

// ParseJson 将json字符串解析为map
func ParseJson(jsonStr string) map[string]interface{} {
	var wecomResponse map[string]interface{}
	if string(jsonStr) != "" {
		err := json.Unmarshal([]byte(string(jsonStr)), &wecomResponse)
		if err != nil {
			log.Println("生成json字符串错误")
		}
	}
	return wecomResponse
}

// GetRemoteToken 从企业微信服务端API获取access_token，存在redis服务则缓存
func GetRemoteToken(corpId, appSecret string) string {
	getTokenUrl := fmt.Sprintf(GetTokenApi, corpId, appSecret)
	log.Println("getTokenUrl==>", getTokenUrl)
	resp, err := http.Get(getTokenUrl)
	if err != nil {
		log.Println(err)
	}
	defer resp.Body.Close()
	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
	}
	tokenResponse := ParseJson(string(respData))
	log.Println("企业微信获取access_token接口返回==>", tokenResponse)
	accessToken := tokenResponse[RedisTokenKey].(string)

	if RedisStat == "ON" {
		log.Println("prepare to set redis key")
		rdb := RedisClient()
		// access_token有效时间为7200秒(2小时)
		set, err := rdb.SetNX(ctx, RedisTokenKey, accessToken, 7000*time.Second).Result()
		log.Println(set)
		if err != nil {
			log.Println(err)
		}
	}
	return accessToken
}

// RedisClient redis客户端
func RedisClient() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     RedisAddr,
		Password: RedisPassword, // no password set
		DB:       0,             // use default DB
	})
	return rdb
}

// PostMsg 推送消息
func PostMsg(postData JsonData, postUrl string) string {
	postJson, _ := json.Marshal(postData)
	log.Println("postJson ", string(postJson))
	log.Println("postUrl ", postUrl)
	msgReq, err := http.NewRequest("POST", postUrl, bytes.NewBuffer(postJson))
	if err != nil {
		log.Println(err)
	}
	msgReq.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(msgReq)
	if err != nil {
		log.Fatalln("企业微信发送应用消息接口报错==>", err)
	}
	defer msgReq.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	mediaResp := ParseJson(string(body))
	log.Println("企业微信发送应用消息接口返回==>", mediaResp)
	return string(body)
}

// UploadMedia  上传临时素材并返回mediaId
func UploadMedia(msgType string, req *http.Request, accessToken string) (string, float64) {
	// 企业微信图片上传不能大于2M
	_ = req.ParseMultipartForm(2 << 20)
	imgFile, imgHeader, err := req.FormFile("media")
	log.Printf("文件大小==>%d字节", imgHeader.Size)
	if err != nil {
		log.Fatalln("图片文件出错==>", err)
		// 自定义code无效的图片文件
		return "", 400
	}
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)
	if createFormFile, err := writer.CreateFormFile("media", imgHeader.Filename); err == nil {
		readAll, _ := ioutil.ReadAll(imgFile)
		createFormFile.Write(readAll)
	}
	writer.Close()

	uploadMediaUrl := fmt.Sprintf(UploadMediaApi, accessToken, msgType)
	log.Println("uploadMediaUrl==>", uploadMediaUrl)
	newRequest, _ := http.NewRequest("POST", uploadMediaUrl, buf)
	newRequest.Header.Set("Content-Type", writer.FormDataContentType())
	log.Println("Content-Type ", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(newRequest)
	respData, _ := ioutil.ReadAll(resp.Body)
	mediaResp := ParseJson(string(respData))
	log.Println("企业微信上传临时素材接口返回==>", mediaResp)
	if err != nil {
		log.Fatalln("上传临时素材出错==>", err)
		return "", mediaResp["errcode"].(float64)
	} else {
		return mediaResp["media_id"].(string), float64(0)
	}
}

// ValidateToken 判断accessToken是否失效
// true-未失效, false-失效需重新获取
func ValidateToken(errcode interface{}) bool {
	codeTyp := reflect.TypeOf(errcode)
	log.Println("errcode的数据类型==>", codeTyp)
	if !codeTyp.Comparable() {
		log.Printf("type is not comparable: %v", codeTyp)
		return true
	}

	// 如果errcode为42001表明token已失效，则清空redis中的token缓存
	// 已知codeType为float64
	if math.Abs(errcode.(float64)-float64(42001)) < 1e-3 {
		if RedisStat == "ON" {
			log.Printf("token已失效，开始删除redis中的key==>%s", RedisTokenKey)
			rdb := RedisClient()
			rdb.Del(ctx, RedisTokenKey)
			log.Printf("删除redis中的key==>%s完毕", RedisTokenKey)
		}
		log.Println("现需重新获取token")
		return false
	}
	return true
}

// GetAccessToken 获取企业微信的access_token
func GetAccessToken() string {
	accessToken := ""
	if RedisStat == "ON" {
		log.Println("尝试从redis获取token")
		rdb := RedisClient()
		value, err := rdb.Get(ctx, RedisTokenKey).Result()
		if err == redis.Nil {
			log.Println("access_token does not exist, need get it from remote API")
		}
		accessToken = value
	}
	if accessToken == "" {
		log.Println("get access_token from remote API")
		accessToken = GetRemoteToken(WecomCid, WecomSecret)
	} else {
		log.Println("get access_token from redis")
	}
	return accessToken
}

// InitJsonData 初始化Json公共部分数据
func InitJsonData(msgType string) JsonData {
	return JsonData{
		ToUser:                 WecomToUid,
		AgentId:                WecomAid,
		MsgType:                msgType,
		DuplicateCheckInterval: 600,
	}
}
func HasContentType(r *http.Request, mimetype string) bool {
	contentType := r.Header.Get("Content-type")
	if contentType == "" {
		return mimetype == "application/octet-stream"
	}

	for _, v := range strings.Split(contentType, ",") {
		t, _, err := mime.ParseMediaType(v)
		if err != nil {
			break
		}
		if t == mimetype {
			return true
		}
	}
	return false
}

const (
	ContentTypeBinary   = "application/octet-stream"
	ContentTypeForm     = "application/x-www-form-urlencoded"
	ContentTypeFormData = "multipart/form-data"
	ContentTypeJSON     = "application/json"
	ContentTypeHTML     = "text/html; charset=utf-8"
	ContentTypeText     = "text/plain; charset=utf-8"
)

// 主函数入口
func main() {
	// 设置日志内容显示文件名和行号
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	wecomChan := func(res http.ResponseWriter, req *http.Request) {

		var msgContent, msgType, sendkey string

		// 从 Header 获取 sendkey 和 msg_type，作为最后的后备选项
		headerSendKey := req.Header.Get("sendkey")
		headerMsgType := req.Header.Get("msg_type")

		// 首先检查 URL 查询参数，适用于 GET 和 POST
		queryMsg := req.URL.Query().Get("msg")
		queryMsgType := req.URL.Query().Get("msg_type")
		querySendKey := req.URL.Query().Get("sendkey")

		// 检查 POST 请求体
		if req.Method == http.MethodPost {
			if HasContentType(req, ContentTypeJSON) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					http.Error(res, "Failed to read request body", http.StatusBadRequest)
					return
				}
				jsonBody := ParseJson(string(body))
				if err != nil {
					http.Error(res, "Failed to parse JSON", http.StatusBadRequest)
					return
				}
				if msg, ok := jsonBody["msg"].(string); ok && msg != "" {
					msgContent = msg
				}
				if jsonMsgType, ok := jsonBody["msg_type"].(string); ok && jsonMsgType != "" {
					msgType = jsonMsgType
				}
				if jsonSendkey, ok := jsonBody["sendkey"].(string); ok && jsonSendkey != "" {
					sendkey = jsonSendkey
				}
			} else if HasContentType(req, ContentTypeFormData) {
				err := req.ParseMultipartForm(100 * 1024 * 1024 * 8)
				if err != nil {
					http.Error(res, "Failed to parse form data", http.StatusBadRequest)
					return
				}
				if formMsg := req.FormValue("msg"); formMsg != "" {
					msgContent = formMsg
				}
				if formMsgType := req.FormValue("msg_type"); formMsgType != "" {
					msgType = formMsgType
				}
				if formSendkey := req.FormValue("sendkey"); formSendkey != "" {
					sendkey = formSendkey
				}
			}
		}

		// 如果 msgContent 在 POST 数据中未被设置，检查 GET 查询参数
		if msgContent == "" {
			msgContent = queryMsg
		}
		if msgType == "" {
			msgType = queryMsgType
		}
		if sendkey == "" {
			sendkey = querySendKey
		}

		// 最后检查 Header，如果以上都没有设置
		if sendkey == "" {
			sendkey = headerSendKey
		}
		if msgType == "" {
			msgType = headerMsgType
		}

		// 验证 sendkey 是否存在
		if sendkey == "" {
			http.Error(res, "{\"errcode\":0,\"errmsg\":\"sendkey is required\"}", http.StatusBadRequest)
		}

		// 准备发送应用消息所需参数
		postData := InitJsonData(msgType)

		// 获取token
		accessToken := GetAccessToken()
		// 默认token有效
		tokenValid := true

		// 默认mediaId为空
		mediaId := ""
		if msgType == "image" {
			log.Println("消息是图片")
			// token有效则跳出循环继续执行，否则重试3次
			for i := 0; i <= 3; i++ {
				var errcode float64
				mediaId, errcode = UploadMedia(msgType, req, accessToken)
				log.Printf("企业微信上传临时素材接口返回的media_id==>[%s], errcode==>[%f]\n", mediaId, errcode)
				tokenValid = ValidateToken(errcode)
				if tokenValid {
					break
				}

				accessToken = GetAccessToken()
			}
			postData.Image = Pic{
				MediaId: mediaId,
			}
		}

		if msgType == "markdown" {
			postData.Markdown = Markdown{
				Content: msgContent,
			}
		}
		if msgType == "text" {
			postData.Text = Msg{
				Content: msgContent,
			}
		}

		postStatus := ""
		for i := 0; i <= 3; i++ {
			sendMessageUrl := fmt.Sprintf(SendMessageApi, accessToken)
			postStatus = PostMsg(postData, sendMessageUrl)
			postResponse := ParseJson(postStatus)
			errcode := postResponse["errcode"]
			log.Println("发送应用消息接口返回errcode==>", errcode)
			tokenValid = ValidateToken(errcode)
			// token有效则跳出循环继续执行，否则重试3次
			if tokenValid {
				break
			}
			// 刷新token
			accessToken = GetAccessToken()
		}

		res.Header().Set("Content-type", "application/json")
		_, _ = res.Write([]byte(postStatus))
	}

	http.HandleFunc("/wecomchan", wecomChan)
	http.HandleFunc("/callback", WecomCallback)
	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		_, _ = res.Write([]byte("Wecomchan is running"))
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
