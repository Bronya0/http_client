package main

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// ConfigItem 定义配置项结构
type ConfigItem struct {
	Name        string                 `yaml:"name"`
	Method      string                 `yaml:"method"`
	URL         string                 `yaml:"url"`
	QueryParams map[string]string      `yaml:"query_params,omitempty"`
	PostData    map[string]interface{} `yaml:"post_data,omitempty"`
}

func loadConfig(filename string) ([]ConfigItem, error) {
	var configs []ConfigItem
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(data, &configs)
	if err != nil {
		return nil, err
	}
	return configs, nil
}

func configHandler(w http.ResponseWriter, r *http.Request) {
	configs, err := loadConfig("requests.yaml")
	if err != nil {
		http.Error(w, "Failed to load config", http.StatusInternalServerError)
		return
	}
	err = json.NewEncoder(w).Encode(configs)
	if err != nil {
		return
	}
}

func sendRequestHandler(w http.ResponseWriter, r *http.Request) {
	var requestData struct {
		ID   int    `json:"id"`
		Data string `json:"data"` // 假设POST请求的数据直接作为字符串处理，实际情况可能需要解码JSON等
	}
	err := json.NewDecoder(r.Body).Decode(&requestData)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	configs, err := loadConfig("requests.yaml")
	if err != nil || requestData.ID < 0 || requestData.ID >= len(configs) {
		http.Error(w, "Invalid request configuration", http.StatusBadRequest)
		return
	}

	config := configs[requestData.ID]

	var requestBody []byte
	if config.Method == "POST" {
		// 处理POST数据，这里简单地假设是JSON格式
		requestBody, err = json.Marshal(requestData.Data)
		if err != nil {
			http.Error(w, "Failed to encode request body", http.StatusInternalServerError)
			return
		}
	}

	// 执行HTTP请求
	resp, err := executeRequest(config.Method, config.URL, requestBody, config.QueryParams)
	if err != nil {
		http.Error(w, "Request failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	defer resp.Body.Close()

	// 将响应体返回给前端，这里简化处理，实际可能需要更细致的错误和数据处理
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}
	_, err = w.Write(bodyBytes)
	if err != nil {
		return
	}
}

// 新增执行HTTP请求的辅助函数
func executeRequest(method, urlStr string, requestBody []byte, queryParams map[string]string) (*http.Response, error) {
	var req *http.Request
	var err error

	// 创建请求
	if method == "GET" && queryParams != nil {
		urlParsed, err := url.Parse(urlStr)
		if err != nil {
			return nil, err
		}
		q := urlParsed.Query()
		for k, v := range queryParams {
			q.Set(k, v)
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

	// 设置请求头（如果需要）
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	return client.Do(req)
}

func main() {
	http.HandleFunc("/config", configHandler)

	// 配置静态文件服务
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./template"))))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseFiles("./template/index.html"))
		configs, _ := loadConfig("./config/requests.yaml") // 简化处理，实际应用中应检查错误
		err := tmpl.Execute(w, configs)
		if err != nil {
			return
		}
	})
	http.HandleFunc("/send-request", sendRequestHandler)

	log.Println("Starting server on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
