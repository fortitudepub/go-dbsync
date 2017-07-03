package main

import (
	"./myutil"

	"fmt"
	"github.com/BurntSushi/toml"
	"gopkg.in/kataras/iris.v6"
	"gopkg.in/kataras/iris.v6/adaptors/httprouter"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type goIpAllowConfig struct {
	Envs                []string // 环境
	Mobiles             []string // 可以用于短信验证码校验的手机号码
	MobileTags          []string // 手机号码对应的人员标记，比如King，ZCX
	ListenPort          int      // 监听端口
	SendCaptcha         string   // 短信发送URL
	VerifyCaptcha       string   // 短信校验URL
	UpdateFirewallShell string   // 更新防火墙IP脚本
}

func readIpAllowConfig() goIpAllowConfig {
	fpath := "go-ip-allow.toml"
	if len(os.Args) > 1 {
		fpath = os.Args[1]
	}

	ipAllowConfig := goIpAllowConfig{}
	if _, err := toml.DecodeFile(fpath, &ipAllowConfig); err != nil {
		myutil.CheckErr(err)
	}

	return ipAllowConfig
}

var g_config goIpAllowConfig

func main() {
	g_config = readIpAllowConfig()

	app := iris.New()
	app.Adapt(httprouter.New())

	app.Get("/", goIpAllowIndexHandler)    // 首页
	app.Post("/smsCode", smsCodeHandler)   // 发送验证码
	app.Post("/ipAllow", goIpAllowHandler) // 设置IP权限
	app.Listen(":" + strconv.Itoa(g_config.ListenPort))
}

func goIpAllowHandler(ctx *iris.Context) {
	officeIp := ctx.FormValue("officeIp")
	smsCode := ctx.FormValue("smsCode")
	envs := ctx.FormValue("envs")
	fmt.Println("smsCode:", smsCode, ",officeIp:", officeIp, ",env:", envs)

	alreadyAllowed, allowedIps := isIpAlreadyAllowed(envs, officeIp)
	if alreadyAllowed {
		ctx.WriteString(officeIp + `已设置，请不要重复设置`)
		return
	}

	mobile, _ := ioutil.ReadFile(officeIp + ".mobile")
	// curl -d "mobile=15951771111&path=iplogin&captcha=3232" http://127.0.0.1:8020/v1/notify/verify-captcha
	out, err := exec.Command("curl", "-d", `mobile=`+string(mobile)+
		`&path=iplogin&captcha=`+smsCode, g_config.VerifyCaptcha).Output()
	if err != nil {
		ctx.WriteString(`设置失败，发送短信错误` + err.Error())
		return
	} else {
		curlOut := string(out)
		fmt.Println(curlOut)
		if "true" != strings.TrimSpace(curlOut) {
			ctx.WriteString(`设置失败，发送短信返回` + curlOut)
			return
		}
	}

	if allowedIps == "" {
		allowedIps = officeIp
	} else {
		allowedIps += "," + officeIp
	}

	out, err = exec.Command("/bin/bash", g_config.UpdateFirewallShell, envs, allowedIps).Output()
	if err != nil {
		ctx.WriteString(`设置失败，执行SHELL错误` + err.Error())
		return
	}

	shellOut := string(out)
	fmt.Println(shellOut)
	writeAllowIpFile(envs, allowedIps)

	os.Remove(officeIp + ".mobile")
	ctx.WriteString(`设置成功`)
}

func smsCodeHandler(ctx *iris.Context) {
	ctx.Header().Set("Content-Type", "application/json")

	officeIp := ctx.FormValue("officeIp")
	envs := ctx.FormValue("envs")
	smsTarget := ctx.FormValue("smsTarget")

	if ok, _ := isIpAlreadyAllowed(envs, officeIp); ok {
		ctx.WriteString(`{"ok":false,"msg":"IP已设置，请不要重复设置"}`)
		return
	}

	smsMobile, smsMobileTag := parseSmsMobile(smsTarget)

	ioutil.WriteFile(officeIp+".mobile", []byte(smsMobile), 0644)
	out, err := exec.Command("curl", "-d", `mobile=`+smsMobile+
		`&templateId=1020481&path=iplogin&validMinutes=15`, g_config.SendCaptcha).Output()
	if err != nil {
		fmt.Println("err:" + err.Error())
		ctx.WriteString(`{"ok":false,"msg":"` + err.Error() + `"}`)
	} else {
		fmt.Println("out:" + string(out))
		ctx.WriteString(`{"ok":true,"tag":"` + smsMobileTag + `"}`)
	}
}

func parseSmsMobile(smsTarget string) (smsMobile, smsMobileTag string) {
	for index, mobile := range g_config.Mobiles {
		if mobile == smsTarget {
			return g_config.Mobiles[index], g_config.MobileTags[index]
		}
	}

	randMobileIndex := rand.Intn(len(g_config.Mobiles))
	// curl -d "mobile=18512345678&templateId=1020481&path=iplogin&validMinutes=15" http://127.0.0.1:8020/v1/notify/send-captcha
	return g_config.Mobiles[randMobileIndex], g_config.MobileTags[randMobileIndex]
}

func isIpAlreadyAllowed(envs, ip string) (bool, string) {
	content, err := ioutil.ReadFile(envs + "-AllowIps.txt")
	if err != nil {
		return false, ""
	}

	strContent := string(content)
	alreadyAllowed := strings.Contains(strContent, ip)
	return alreadyAllowed, strContent
}

func writeAllowIpFile(env, content string) {
	ioutil.WriteFile(env+"-AllowIps.txt", []byte(content), 0644)
}

func goIpAllowIndexHandler(ctx *iris.Context) {
	envCheckboxes := ""
	for _, env := range g_config.Envs {
		envCheckboxes += fmt.Sprintf("<input class='env' type='checkbox' value='%v'>%v</input><br/>", env, env)
	}
	ctx.WriteString(`
<html>
<body>
<div style="text-align: center;white-space:nowrap;">
<br/>
请关闭所有代理，防止IP识别错误。<br/>请检查以下显示的IP是：
<span id='myip'></span>
<br/>
<iframe id="iframe" src="http://1212.ip138.com/ic.asp" rel="nofollow" frameborder="0" scrolling="no"
 style="width:100%;height:30px"></iframe>
<br/><div style="width: 200px;margin: 0 auto;text-align: left;">` + envCheckboxes + `</div>
<br/> 验证码发送到：<input type="input" id="smsTarget" style="width:60px"/>
<label style="font-size: 14px;">(输入人名)</label>
<br/> 请输入验证码：<input type="input" id="smsCode" style="width:60px"/>
<button onclick="sendSmsCode()" style="font-size: 14px;">发验证码</button>
<span id="smsCodeTarget"></span>
<br/><br/>
<button onclick="setIpAllow()" style="font-size: 14px; padding: 3px 106px; ">设置</button>
</div>

</body>
<script>
function setIpAllow() {
	minAjax({
		url:"/ipAllow",
		type:"POST",
		data: {
			envs: getCheckedValues('env'),
			officeIp:$('myip').innerText,
			smsCode:$('smsCode').value
		},
		success: function(data) {
			alert(data)
		}
	})
}

function sendSmsCode() {
	minAjax({
		url:"/smsCode",
		type:"POST",
		data: {
			envs: getCheckedValues('env'),
			officeIp: $('myip').innerText,
			smsTarget: $('smsTarget').value
		},
		success: function(rsp){
			var data = JSON.parse(rsp)
			if (data.ok) {
				$('smsCodeTarget').innerText = "已发到" + data.tag +"，5分内有效";
				$('smsCode').focus()
			} else {
				alert(data.msg)
			}
		}
	})
}

function getCheckedValues(checkboxClass) {
	var checkedValue = []
	var inputElements = document.getElementsByClassName(checkboxClass)
	for(var i = 0; inputElements[i]; ++i){
	      if(inputElements[i].checked){
		   checkedValue.push(inputElements[i].value)
	      }
	}
	return checkedValue.join(',')
}

function $(id) {
	return document.getElementById(id)
}

/*|--(A Minimalistic Pure JavaScript Header for Ajax POST/GET Request )--|
  |--Author : flouthoc (gunnerar7@gmail.com)(http://github.com/flouthoc)--|
  */
function initXMLhttp() {
	if (window.XMLHttpRequest) { // code for IE7,firefox chrome and above
		return new XMLHttpRequest()
	} else { // code for Internet Explorer
		return new ActiveXObject("Microsoft.XMLHTTP")
	}
}

function minAjax(config) {
	/*
	Config Structure
	url:"reqesting URL"
	type:"GET or POST"
	method: "(OPTIONAL) True for async and False for Non-async | By default its Async"
	data: "(OPTIONAL) another Nested Object which should contains reqested Properties in form of Object Properties"
	success: "(OPTIONAL) Callback function to process after response | function(data,status)"
	*/

	if (!config.method) {
		config.method = true
	}

	var xmlhttp = initXMLhttp()
	xmlhttp.onreadystatechange = function() {
		if (xmlhttp.readyState == 4 && xmlhttp.status == 200) {
			config.success(xmlhttp.responseText, xmlhttp.readyState)
		}
	}

	var sendString = [], sendData = config.data
	if ( typeof sendData === "string" ){
		var tmpArr = String.prototype.split.call(sendData,'&')
		for(var i = 0, j = tmpArr.length; i < j; i++){
			var datum = tmpArr[i].split('=')
			sendString.push(encodeURIComponent(datum[0]) + "=" + encodeURIComponent(datum[1]))
		}
	} else if( typeof sendData === 'object' && !( sendData instanceof String || (FormData && sendData instanceof FormData) ) ){
		for (var k in sendData) {
			var datum = sendData[k]
			if ( Object.prototype.toString.call(datum) == "[object Array]" ){
				for(var i = 0, j = datum.length; i < j; i++) {
					sendString.push(encodeURIComponent(k) + "[]=" + encodeURIComponent(datum[i]))
				}
			} else {
				sendString.push(encodeURIComponent(k) + "=" + encodeURIComponent(datum))
			}
		}
	}
	sendString = sendString.join('&')

	if (config.type == "GET") {
		xmlhttp.open("GET", config.url + "?" + sendString, config.method)
		xmlhttp.send()
	} else if (config.type == "POST") {
		xmlhttp.open("POST", config.url, config.method)
		xmlhttp.setRequestHeader("Content-type", "application/x-www-form-urlencoded")
		xmlhttp.send(sendString)
	}
}

minAjax({
		url:"http://icanhazip.com",
		type:"GET",
		data:{},
		success: function(data){
			$('myip').innerText = data.trim()
		}
	})
</script>
</html>
	`)
}
