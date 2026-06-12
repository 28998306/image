package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"web2img/internal/reg"
)

// App struct
type App struct {
	ctx context.Context
	reg *reg.Manager
}

type GenerationRequest struct {
	Mode       string   `json:"mode"`
	Prompt     string   `json:"prompt"`
	Model      string   `json:"model"`
	Quality    string   `json:"quality"`
	Size       string   `json:"size"`
	Count      int      `json:"count"`
	Duration   int      `json:"duration"`
	Ratio      string   `json:"ratio"`
	ImagePath  string   `json:"imagePath,omitempty"`
	ImagePaths []string `json:"imagePaths,omitempty"`
	MaskPath   string   `json:"maskPath,omitempty"`
}

type GenerationResponse struct {
	JobID      string   `json:"jobId"`
	Status     string   `json:"status"`
	Progress   int      `json:"progress"`
	RetryAfter int      `json:"retryAfter"`
	ImageURLs  []string `json:"imageUrls"`
	VideoURLs  []string `json:"videoUrls"`
	CoverURLs  []string `json:"coverUrls"`
	Message    string   `json:"message"`
}

type HistoryItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Mode      string `json:"mode"`
	Prompt    string `json:"prompt"`
	Model     string `json:"model"`
	Quality   string `json:"quality"`
	Size      string `json:"size"`
	ImageURL  string `json:"imageUrl"`
	VideoURL  string `json:"videoUrl"`
	CoverURL  string `json:"coverUrl"`
	LocalPath string `json:"localPath"`
	CreatedAt string `json:"createdAt"`
}

type AppConfig struct {
	Provider            string `json:"provider"`
	BaseURL             string `json:"baseUrl"`
	APIKey              string `json:"apiKey"`
	DefaultModel        string `json:"defaultModel"`
	DefaultImageModel   string `json:"defaultImageModel"`
	DefaultQuality      string `json:"defaultQuality"`
	DefaultSize         string `json:"defaultSize"`
	DefaultVideoModel   string `json:"defaultVideoModel"`
	DefaultDuration     int    `json:"defaultDuration"`
	DefaultRatio        string `json:"defaultRatio"`
	DefaultVideoQuality string `json:"defaultVideoQuality"`
	AsyncImages         bool   `json:"asyncImages"`
	AsyncVideos         bool   `json:"asyncVideos"`
	CallbackURL         string `json:"callbackUrl"`
	OutputDir           string `json:"outputDir"`
}

type ImageModel struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Price1K     string              `json:"price1k"`
	Price2K     string              `json:"price2k"`
	Price4K     string              `json:"price4k"`
	Description string              `json:"description"`
	Sizes       map[string][]string `json:"sizes"`
}

type VideoModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Pricing     string `json:"pricing"`
}

type apiAsset struct {
	URL        string `json:"url"`
	CoverURL   string `json:"cover_url"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	DurationMS int    `json:"duration_ms"`
}

type apiResult struct {
	Data []apiAsset `json:"data"`
}

type apiError struct {
	Message string `json:"message"`
}

type apiImageResponse struct {
	ID         string      `json:"id"`
	TaskID     string      `json:"task_id"`
	Status     string      `json:"status"`
	Progress   int         `json:"progress"`
	RetryAfter int         `json:"retry_after"`
	Data       []apiAsset  `json:"data"`
	Result     *apiResult  `json:"result"`
	Error      interface{} `json:"error"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dataDir := filepath.Join(filepath.Dir(configFilePath()), "reg")
	if mgr, err := reg.NewManager(dataDir); err != nil {
		log.Printf("reg manager init failed: %v", err)
	} else {
		a.reg = mgr
	}
}

func (a *App) GenerateTextToImage(request GenerationRequest) GenerationResponse {
	request.Mode = "text"
	return a.submitImageGeneration("/images/generations", request)
}

func (a *App) GenerateImageToImage(request GenerationRequest) GenerationResponse {
	request.Mode = "image"
	return a.submitImageGeneration("/images/generations", request)
}

func (a *App) GenerateVideo(request GenerationRequest) GenerationResponse {
	request.Mode = "video"
	return a.submitVideoGeneration(request)
}

func (a *App) GenerateInpaint(request GenerationRequest) GenerationResponse {
	request.Mode = "inpaint"
	return a.submitImageGeneration("/images/edits", request)
}

func (a *App) PollImageTask(taskID string) GenerationResponse {
	config := loadAppConfig()
	if strings.TrimSpace(config.APIKey) == "" {
		return GenerationResponse{Status: "failed", Message: "请先在系统设置中保存 gpt2api API Key"}
	}
	if strings.TrimSpace(taskID) == "" {
		return GenerationResponse{Status: "failed", Message: "任务 ID 不能为空"}
	}

	endpoint := strings.TrimRight(config.BaseURL, "/") + "/images/generations/" + taskID
	httpRequest, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	httpRequest.Header.Set("Authorization", "Bearer "+config.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(httpRequest)
	if err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	defer response.Body.Close()

	var apiResponse apiImageResponse
	if err := json.NewDecoder(response.Body).Decode(&apiResponse); err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return GenerationResponse{JobID: taskID, Status: "failed", Message: apiErrorMessage(apiResponse.Error, response.Status)}
	}

	return generationResponseFromAPI(apiResponse, taskID)
}

func (a *App) PollVideoTask(taskID string) GenerationResponse {
	config := loadAppConfig()
	if strings.TrimSpace(config.APIKey) == "" {
		return GenerationResponse{Status: "failed", Message: "请先在系统设置中保存 gpt2api API Key"}
	}
	if strings.TrimSpace(taskID) == "" {
		return GenerationResponse{Status: "failed", Message: "任务 ID 不能为空"}
	}

	endpoint := strings.TrimRight(config.BaseURL, "/") + "/video/generations/" + taskID
	httpRequest, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	httpRequest.Header.Set("Authorization", "Bearer "+config.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(httpRequest)
	if err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	defer response.Body.Close()

	var apiResponse apiImageResponse
	if err := json.NewDecoder(response.Body).Decode(&apiResponse); err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return GenerationResponse{JobID: taskID, Status: "failed", Message: apiErrorMessage(apiResponse.Error, response.Status)}
	}

	return generationResponseFromAPI(apiResponse, taskID)
}

func (a *App) SaveImage(path string, data string) bool {
	// Real file persistence will be added when the provider response format is finalized.
	return path != "" && data != ""
}

func (a *App) OpenOutputDir() bool {
	dir := outputDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false
	}
	return exec.Command("explorer", dir).Start() == nil
}

func (a *App) OpenLocalFile(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start() == nil
}

func (a *App) SaveRemoteFile(assetURL string, title string) string {
	if strings.TrimSpace(assetURL) == "" {
		return ""
	}

	response, err := http.Get(assetURL)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return ""
	}

	dir := outputDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ""
	}

	filename := safeFilename(firstNonEmpty(title, "web2img-output")) + "-" + time.Now().Format("20060102-150405") + fileExtension(assetURL, response.Header.Get("Content-Type"))
	path := filepath.Join(dir, filename)
	file, err := os.Create(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	if _, err := io.Copy(file, response.Body); err != nil {
		return ""
	}
	return path
}

func (a *App) LoadHistory() []HistoryItem {
	return loadHistory()
}

func (a *App) SaveHistory(item HistoryItem) bool {
	return saveHistoryItem(item) == nil
}

func (a *App) GetAppConfig() AppConfig {
	return loadAppConfig()
}

func (a *App) SaveAppConfig(config AppConfig) bool {
	return saveAppConfig(config) == nil
}

func (a *App) GetImageModels() []ImageModel {
	return gpt2apiImageModels()
}

func (a *App) GetVideoModels() []VideoModel {
	return gpt2apiVideoModels()
}

func (a *App) submitImageGeneration(path string, request GenerationRequest) GenerationResponse {
	config := loadAppConfig()
	if config.Provider != "gpt2api" || strings.TrimSpace(config.APIKey) == "" {
		return GenerationResponse{Status: "failed", Message: "请先在系统设置中保存 gpt2api API Key"}
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return GenerationResponse{Status: "failed", Message: "提示词不能为空"}
	}
	if (request.Mode == "image" || request.Mode == "inpaint") && strings.TrimSpace(request.ImagePath) == "" {
		return GenerationResponse{Status: "failed", Message: "图生图和图片编辑需要先上传参考图"}
	}

	payload := map[string]interface{}{
		"model":   firstNonEmpty(request.Model, config.DefaultImageModel),
		"prompt":  request.Prompt,
		"n":       maxInt(request.Count, 1),
		"size":    normalizeSize(firstNonEmpty(request.Size, config.DefaultSize)),
		"quality": firstNonEmpty(request.Quality, config.DefaultQuality),
		"async":   config.AsyncImages,
	}

	if config.CallbackURL != "" {
		payload["callback_url"] = config.CallbackURL
	}
	if len(request.ImagePaths) > 0 {
		payload["images"] = request.ImagePaths
	} else if request.ImagePath != "" {
		payload["image"] = request.ImagePath
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}

	endpoint := strings.TrimRight(config.BaseURL, "/") + path
	httpRequest, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	httpRequest.Header.Set("Authorization", "Bearer "+config.APIKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Idempotency-Key", "web2img-"+randomID())

	client := &http.Client{Timeout: 10 * time.Minute}
	response, err := client.Do(httpRequest)
	if err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	defer response.Body.Close()

	var apiResponse apiImageResponse
	if err := json.NewDecoder(response.Body).Decode(&apiResponse); err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}

	if response.StatusCode >= http.StatusBadRequest {
		return GenerationResponse{Status: "failed", Message: apiErrorMessage(apiResponse.Error, response.Status)}
	}

	return generationResponseFromAPI(apiResponse, "")
}

func (a *App) submitVideoGeneration(request GenerationRequest) GenerationResponse {
	config := loadAppConfig()
	if config.Provider != "gpt2api" || strings.TrimSpace(config.APIKey) == "" {
		return GenerationResponse{Status: "failed", Message: "请先在系统设置中保存 gpt2api API Key"}
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return GenerationResponse{Status: "failed", Message: "提示词不能为空"}
	}

	payload := map[string]interface{}{
		"model":    firstNonEmpty(request.Model, config.DefaultVideoModel),
		"prompt":   request.Prompt,
		"duration": maxInt(request.Duration, config.DefaultDuration),
		"ratio":    firstNonEmpty(request.Ratio, config.DefaultRatio),
		"quality":  firstNonEmpty(request.Quality, config.DefaultVideoQuality),
		"async":    config.AsyncVideos,
	}

	if config.CallbackURL != "" {
		payload["callback_url"] = config.CallbackURL
	}
	if len(request.ImagePaths) > 0 {
		payload["images"] = request.ImagePaths
	} else if request.ImagePath != "" {
		payload["image"] = request.ImagePath
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}

	endpoint := strings.TrimRight(config.BaseURL, "/") + "/video/generations"
	httpRequest, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	httpRequest.Header.Set("Authorization", "Bearer "+config.APIKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Idempotency-Key", "web2img-video-"+randomID())

	client := &http.Client{Timeout: 10 * time.Minute}
	response, err := client.Do(httpRequest)
	if err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	defer response.Body.Close()

	var apiResponse apiImageResponse
	if err := json.NewDecoder(response.Body).Decode(&apiResponse); err != nil {
		return GenerationResponse{Status: "failed", Message: err.Error()}
	}
	if response.StatusCode >= http.StatusBadRequest {
		return GenerationResponse{Status: "failed", Message: apiErrorMessage(apiResponse.Error, response.Status)}
	}

	return generationResponseFromAPI(apiResponse, "")
}

func mockGenerationResponse(request GenerationRequest) GenerationResponse {
	return GenerationResponse{
		JobID:      "mock-" + time.Now().Format("20060102150405"),
		Status:     "queued",
		Progress:   0,
		RetryAfter: 2,
		ImageURLs:  []string{},
		Message:    "Mock generation queued for " + request.Mode,
	}
}

func defaultAppConfig() AppConfig {
	return AppConfig{
		Provider:            "gpt2api",
		BaseURL:             "https://www.gpt2api.com/v1",
		APIKey:              "",
		DefaultModel:        "nano-banana-pro",
		DefaultImageModel:   "nano-banana-pro",
		DefaultQuality:      "1k",
		DefaultSize:         "1024x1024",
		DefaultVideoModel:   "grok-imagine-video",
		DefaultDuration:     10,
		DefaultRatio:        "16:9",
		DefaultVideoQuality: "hd",
		AsyncImages:         true,
		AsyncVideos:         true,
		CallbackURL:         "",
		OutputDir:           "",
	}
}

func configFilePath() string {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		baseDir = "."
	}
	return filepath.Join(baseDir, "Web2Img AI Studio", "config.json")
}

func historyFilePath() string {
	return filepath.Join(filepath.Dir(configFilePath()), "history.json")
}

func outputDir() string {
	config := loadAppConfig()
	if strings.TrimSpace(config.OutputDir) != "" {
		return config.OutputDir
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(filepath.Dir(configFilePath()), "outputs")
	}
	return filepath.Join(homeDir, "Pictures", "Web2Img AI Studio")
}

func loadAppConfig() AppConfig {
	config := defaultAppConfig()
	data, err := os.ReadFile(configFilePath())
	if err != nil {
		return config
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return defaultAppConfig()
	}
	return normalizeConfig(config)
}

func saveAppConfig(config AppConfig) error {
	config = normalizeConfig(config)
	path := configFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func loadHistory() []HistoryItem {
	data, err := os.ReadFile(historyFilePath())
	if err != nil {
		return []HistoryItem{}
	}

	var history []HistoryItem
	if err := json.Unmarshal(data, &history); err != nil {
		return []HistoryItem{}
	}
	return history
}

func saveHistoryItem(item HistoryItem) error {
	if item.ID == "" {
		item.ID = "history-" + randomID()
	}
	if item.Title == "" {
		item.Title = "生成结果"
	}
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().Format(time.RFC3339)
	}

	history := append([]HistoryItem{item}, loadHistory()...)
	if len(history) > 200 {
		history = history[:200]
	}

	path := historyFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func normalizeConfig(config AppConfig) AppConfig {
	defaults := defaultAppConfig()
	config.Provider = firstNonEmpty(config.Provider, defaults.Provider)
	config.BaseURL = strings.TrimRight(firstNonEmpty(config.BaseURL, defaults.BaseURL), "/")
	config.DefaultImageModel = firstNonEmpty(config.DefaultImageModel, config.DefaultModel, defaults.DefaultImageModel)
	config.DefaultModel = firstNonEmpty(config.DefaultModel, config.DefaultImageModel)
	config.DefaultQuality = strings.ToLower(firstNonEmpty(config.DefaultQuality, defaults.DefaultQuality))
	config.DefaultSize = normalizeSize(firstNonEmpty(config.DefaultSize, defaults.DefaultSize))
	config.DefaultVideoModel = firstNonEmpty(config.DefaultVideoModel, defaults.DefaultVideoModel)
	config.DefaultDuration = maxInt(config.DefaultDuration, defaults.DefaultDuration)
	config.DefaultRatio = firstNonEmpty(config.DefaultRatio, defaults.DefaultRatio)
	config.DefaultVideoQuality = strings.ToLower(firstNonEmpty(config.DefaultVideoQuality, defaults.DefaultVideoQuality))
	return config
}

func gpt2apiImageModels() []ImageModel {
	return []ImageModel{
		{
			ID:          "nano-banana-pro",
			Name:        "Nano Banana Pro",
			Price1K:     "0.1",
			Price2K:     "0.15",
			Price4K:     "0.2",
			Description: "高保真出图，稳定性最好，适合高质量交付。",
			Sizes:       bananaSizes(),
		},
		{
			ID:          "nano-banana-v2",
			Name:        "Nano Banana V2",
			Price1K:     "0.08",
			Price2K:     "0.12",
			Price4K:     "0.15",
			Description: "上一代改进版，速度和价格折中。",
			Sizes:       bananaSizes(),
		},
		{
			ID:          "nano-banana",
			Name:        "Nano Banana",
			Price1K:     "0.08",
			Price2K:     "0.12",
			Price4K:     "0.15",
			Description: "基础款，成本低，适合草图和批量出图。",
			Sizes:       bananaSizes(),
		},
		{
			ID:          "gpt-image-2",
			Name:        "GPT Image 2",
			Price1K:     "0.08",
			Price2K:     "0.1",
			Price4K:     "0.15",
			Description: "通用图像模型，支持 1K / 2K / 4K，尺寸表与 Banana 不同。",
			Sizes:       gptImage2Sizes(),
		},
	}
}

func gpt2apiVideoModels() []VideoModel {
	return []VideoModel{
		{ID: "grok-imagine-video", Name: "Grok Imagine Video", Pricing: "6s 0.2点 / 10s 0.3点 / 20s 0.4点 / 30s 0.5点", Description: "文生视频 / 图生视频统一入口，支持 6s / 10s / 20s / 30s。"},
		{ID: "sora", Name: "Sora 2", Pricing: "4s 0.3点 / 8s 0.5点 / 12s 0.8点", Description: "高质量视频生成模型。"},
		{ID: "veo3.1", Name: "VEO 3.1", Pricing: "4s 0.3点 / 6s 0.5点 / 8s 0.8点", Description: "VEO 3.1 视频模型。"},
		{ID: "veo3.1-flash", Name: "VEO 3.1 Flash", Pricing: "4s 0.3点 / 6s 0.5点 / 8s 0.8点", Description: "更快的视频生成模型。"},
		{ID: "veo3.1-lite", Name: "VEO 3.1 Lite", Pricing: "4s 0.3点 / 6s 0.5点 / 8s 0.8点", Description: "轻量视频生成模型。"},
	}
}

func bananaSizes() map[string][]string {
	return map[string][]string{
		"1k": []string{"1024x1024", "1264x848", "848x1264", "1152x864", "864x1152", "1152x928", "928x1152", "1376x768", "768x1376", "1584x672"},
		"2k": []string{"2048x2048", "2528x1696", "1696x2528", "2048x1536", "1536x2048", "2304x1856", "1856x2304", "2752x1536", "1536x2752", "3168x1344"},
		"4k": []string{"4096x4096", "5056x3392", "3392x5056", "4784x3584", "3584x4784", "4608x3712", "3712x4608", "5504x3072", "3072x5504", "6336x2688"},
	}
}

func gptImage2Sizes() map[string][]string {
	return map[string][]string{
		"1k": []string{"1024x1024", "1536x1024", "1024x1536", "1152x864", "864x1152", "1120x896", "896x1120", "1280x720", "720x1280", "1456x624"},
		"2k": []string{"2048x2048", "2496x1664", "1664x2496", "2304x1728", "1728x2304", "2240x1792", "1792x2240", "2560x1440", "1440x2560", "3024x1296"},
		"4k": []string{"2480x2480", "3056x2032", "2032x3056", "2880x2160", "2160x2880", "2784x2224", "2224x2784", "3328x1872", "1872x3328", "3808x1632"},
	}
}

func collectImageURLs(response apiImageResponse) []string {
	assets := response.Data
	if response.Result != nil {
		assets = append(assets, response.Result.Data...)
	}

	urls := make([]string, 0, len(assets))
	for _, asset := range assets {
		if asset.URL != "" {
			urls = append(urls, asset.URL)
		}
	}
	return urls
}

func collectVideoURLs(response apiImageResponse) []string {
	assets := response.Data
	if response.Result != nil {
		assets = append(assets, response.Result.Data...)
	}

	urls := make([]string, 0, len(assets))
	for _, asset := range assets {
		if asset.URL != "" {
			urls = append(urls, asset.URL)
		}
	}
	return urls
}

func collectCoverURLs(response apiImageResponse) []string {
	assets := response.Data
	if response.Result != nil {
		assets = append(assets, response.Result.Data...)
	}

	urls := make([]string, 0, len(assets))
	for _, asset := range assets {
		if asset.CoverURL != "" {
			urls = append(urls, asset.CoverURL)
		}
	}
	return urls
}

func generationResponseFromAPI(response apiImageResponse, fallbackTaskID string) GenerationResponse {
	imageURLs := collectImageURLs(response)
	videoURLs := collectVideoURLs(response)
	coverURLs := collectCoverURLs(response)
	jobID := firstNonEmpty(response.TaskID, response.ID, fallbackTaskID, "sync-"+randomID())
	status := firstNonEmpty(response.Status, "succeeded")
	if response.Status == "" && len(imageURLs) == 0 && len(videoURLs) == 0 && jobID != "" {
		status = "queued"
	}
	progress := response.Progress
	if status == "succeeded" && progress == 0 {
		progress = 100
	}

	message := "gpt2api request submitted"
	if response.RetryAfter > 0 {
		message = "gpt2api task queued; retry after " + (time.Duration(response.RetryAfter) * time.Second).String()
	}

	return GenerationResponse{
		JobID:      jobID,
		Status:     status,
		Progress:   progress,
		RetryAfter: response.RetryAfter,
		ImageURLs:  imageURLs,
		VideoURLs:  videoURLs,
		CoverURLs:  coverURLs,
		Message:    message,
	}
}

func apiErrorMessage(value interface{}, fallback string) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		if message, ok := typed["message"].(string); ok && message != "" {
			return message
		}
	case string:
		if typed != "" {
			return typed
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeSize(size string) string {
	return strings.ReplaceAll(strings.TrimSpace(size), " ", "")
}

func maxInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func safeFilename(value string) string {
	replacer := strings.NewReplacer("\\", "-", "/", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-")
	value = strings.TrimSpace(replacer.Replace(value))
	if value == "" {
		return "web2img-output"
	}
	if len([]rune(value)) > 48 {
		return string([]rune(value)[:48])
	}
	return value
}

func fileExtension(assetURL string, contentType string) string {
	parsedURL, err := neturl.Parse(assetURL)
	if err == nil {
		extension := strings.ToLower(filepath.Ext(parsedURL.Path))
		if extension != "" {
			return extension
		}
	}
	contentType = strings.ToLower(contentType)
	switch {
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		return ".jpg"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	case strings.Contains(contentType, "mp4"):
		return ".mp4"
	default:
		return ".bin"
	}
}

func randomID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return time.Now().Format("20060102150405")
	}
	return hex.EncodeToString(bytes)
}
