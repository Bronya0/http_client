package main

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v3"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	configs []ConfigItem
	logger  = InitLogger("./all.log")
)

// ConfigItem 定义配置项结构
type ConfigItem struct {
	Name     string            `yaml:"name"`
	Method   string            `yaml:"method"`
	Download bool              `yaml:"download,omitempty"`
	URL      string            `yaml:"url"`
	Params   map[string]string `yaml:"params,omitempty"`
}

func loadConfig(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(data, &configs)
	if err != nil {
		panic(err)
	}

	var old []string
	for _, item := range configs {
		if slices.Contains(old, item.Name) {
			logger.Error("存在重复配置name：", item.Name)
		} else {
			old = append(old, item.Name)
		}

	}

}

// InitLogger pathFile: 日志全路径
func InitLogger(pathFile string) *zap.SugaredLogger {

	writer := zapcore.AddSync(&lumberjack.Logger{
		Filename:   pathFile, //日志文件的位置 /xxx.log
		MaxSize:    100,      // 在进行切割之前，日志文件的最大大小（以MB为单位）
		MaxBackups: 10,       // 保留旧文件的最大个数
		MaxAge:     120,      //保留旧文件的最大天数(按日期滚动)
		Compress:   true,     //是否压缩/归档旧文件
	})
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02 15:04:05.000")) // 时间格式
	}
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	var encoder = zapcore.NewConsoleEncoder(encoderConfig) // 普通模式，还有json模式
	writer = zapcore.AddSync(writer)
	zapCore := zapcore.NewCore(encoder, writer, zap.InfoLevel)                              // 日志等级下限
	_logger := zap.New(zapCore, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)).Sugar() // error及以上的级别增加堆栈; sugar允许使用f方法
	_logger.Info("日志初始化成功...")
	return _logger
}

func sendRequestHandler(c *gin.Context) {
	var requestData struct {
		Name   string                 `json:"name"`
		Params map[string]interface{} `json:"params;omitempty"` // 假设POST请求的数据直接作为字符串处理，实际情况可能需要解码JSON等
	}

	err := c.ShouldBindBodyWithJSON(&requestData)
	if err != nil {
		panic(err)
	}

	jsonBytes, err := json.Marshal(requestData.Params)
	if err != nil {
		panic(err)

	}

	for _, configItem := range configs {
		if configItem.Name == requestData.Name {
			// 执行HTTP请求
			resp, err := executeRequest(configItem.Method, configItem.URL, jsonBytes, requestData.Params)
			if err != nil {
				c.JSON(http.StatusBadGateway, err.Error())
				return
			}

			// 将响应体返回给前端，这里简化处理，实际可能需要更细致的错误和数据处理
			bodyBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()

			if err != nil {
				c.JSON(http.StatusBadGateway, err.Error())
				return
			}
			// 复制响应头
			for k, vs := range resp.Header {
				for _, v := range vs {
					c.Header(k, v)
				}
			}
			_, err = c.Writer.Write(bodyBytes)

			if err != nil {
				c.JSON(http.StatusBadGateway, err.Error())
				return
			}

		}
	}

}

// 新增执行HTTP请求的辅助函数
func executeRequest(method, urlStr string, requestBody []byte, queryParams map[string]interface{}) (*http.Response, error) {
	var req *http.Request
	var err error

	// 创建请求
	if strings.ToUpper(method) == "GET" && queryParams != nil {
		urlParsed, err := url.Parse(urlStr)
		if err != nil {
			return nil, err
		}
		q := urlParsed.Query()
		for k, v := range queryParams {
			switch v := v.(type) {
			case string:
				q.Set(k, v)
			case int:
				q.Set(k, strconv.Itoa(v))
			}
		}
		urlParsed.RawQuery = q.Encode()
		urlStr = urlParsed.String()
		req, err = http.NewRequest(method, urlStr, nil)
	} else {
		req, err = http.NewRequest(method, urlStr, strings.NewReader(string(requestBody)))
	}
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	return client.Do(req)
}

func main() {

	// 设置 release模式
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = io.Discard
	r := gin.New()

	loadConfig("./config/requests.yaml")

	// 配置静态文件服务
	r.Static("/static", "./template")

	r.GET("/", func(c *gin.Context) {
		tmpl := template.Must(template.ParseFiles("./template/index.html"))
		err := tmpl.Execute(c.Writer, configs)
		if err != nil {
			return
		}
	})

	r.POST("/send-request", sendRequestHandler)

	r.GET("/hello", func(c *gin.Context) {
		c.JSON(200, "hello")
	})

	r.GET("/hello_json", func(c *gin.Context) {
		c.JSON(200, gin.H{"hello": time.Now().Format(time.DateTime)})
	})
	r.POST("/post_json", func(c *gin.Context) {
		var param struct {
			Value2 int    `json:"value2"`
			Value3 string `json:"value3"`
		}

		err := c.ShouldBindJSON(&param)

		if err != nil {
			logger.Warn("请求参数绑定不对：%s", err.Error())
			return
		}

		c.JSON(200, gin.H{"hello": time.Now().Format(time.DateTime),
			"value2": param.Value2,
			"value3": param.Value3,
		})
		return
	})
	r.GET("/download", func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "application/octet-stream")
		c.FileAttachment("./config/requests.yaml", "requests.yaml")
		return
	})

	log.Fatal(r.Run(":8080"))
}
