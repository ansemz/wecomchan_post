package main

import (
	"github.com/go-laoji/wxbizmsgcrypt"
	"net/http"
)

type QueryParams struct {
	MsgSignature string `form:"msg_signature"`
	TimeStamp    string `form:"timestamp"`
	Nonce        string `form:"nonce"`
	EchoStr      string `form:"echostr"`
}

func WecomCallback(res http.ResponseWriter, req *http.Request) {
	wxbiz := wxbizmsgcrypt.NewWXBizMsgCrypt(WecomToken, WecomAesKey, WecomCid, wxbizmsgcrypt.XmlType)

	if req.Method == http.MethodGet {
		var q QueryParams
		q.MsgSignature = req.URL.Query().Get("msg_signature")
		q.TimeStamp = req.URL.Query().Get("timestamp")
		q.Nonce = req.URL.Query().Get("nonce")
		q.EchoStr = req.URL.Query().Get("echostr")

		echoStr, err := wxbiz.VerifyURL(q.MsgSignature, q.TimeStamp, q.Nonce, q.EchoStr)
		if err != nil {
			http.Error(res, err.ErrMsg, http.StatusNotImplemented)
		} else {
			res.Write(echoStr)
		}
	} else {
		// 处理其他请求方法，例如 POST
	}
}
