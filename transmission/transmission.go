package transmission

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	humanize "github.com/dustin/go-humanize"
)

const (
	StatusStopped = iota
	StatusCheckPending
	StatusChecking
	StatusDownloadPending
	StatusDownloading
	StatusSeedPending
	StatusSeeding
)

// Request
type Request struct {
	Method    string    `json:"method"`
	Arguments arguments `json:"arguments"`
}

type arguments struct {
	Ids      []string `json:"ids,omitempty"`
	Fields   []string `json:"fields,omitempty"`
	Filename string   `json:"filename,omitempty"`
}

// Response
type AddTorrentResponse struct {
	Result string `json:"result"`

	Arguments struct {
		TorrentAdded     *TorrentInfo `json:"torrent-added"`
		TorrentDuplicate *TorrentInfo `json:"torrent-duplicate"`
	} `json:"arguments"`
}

type TorrentInfo struct {
	Id         int64  `json:"id"`
	HashString string `json:"hashString"`
	Name       string `json:"name"`
}

type TorrentError struct {
	Error       int    `json:"error"`
	ErrorString string `json:"errorString"`
}

type GetTorrentsResponse struct {
	Result string `json:"result"`

	Arguments struct {
		Torrents []*Torrent `json:"torrents"`
	} `json:"arguments"`
}

type Torrent struct {
	TorrentInfo
	TorrentError

	Eta time.Duration `json:"eta"`

	HaveUnchecked uint64 `json:"haveUnchecked"`
	HaveValid     uint64 `json:"haveValid"`

	LeftUntilDone uint64  `json:"leftUntilDone"`
	PercentDone   float64 `json:"percentDone"`
	RateDownload  uint64  `json:"rateDownload"`
	RateUpload    uint64  `json:"rateUpload"`
	SizeWhenDone  uint64  `json:"sizeWhenDone"`
	Status        int     `json:"status"`
}

func (torrent *Torrent) TorrentStatus() string {
	switch torrent.Status {
	case StatusStopped:
		return "Stopped"
	case StatusCheckPending:
		return "Check waiting"
	case StatusChecking:
		return "Checking"
	case StatusDownloadPending:
		return "Download waiting"
	case StatusDownloading:
		return "Downloading"
	case StatusSeedPending:
		return "Seed waiting"
	case StatusSeeding:
		return "Seeding"
	default:
		return "Unknown"
	}
}

func (torrent *Torrent) ETA() string {
	if torrent.Eta < 0 {
		return "∞"
	}

	return "~\u00A0" + humanize.RelTime(time.Now().Add(time.Second*torrent.Eta), time.Now(), "", "")
}

func (torrent *Torrent) DownloadRate() string {
	return humanize.Bytes(torrent.RateDownload)
}

func (torrent *Torrent) UploadRate() string {
	return humanize.Bytes(torrent.RateUpload)
}

func (torrent *Torrent) Downloaded() string {
	return humanize.Bytes(torrent.HaveValid + torrent.HaveUnchecked)
}

func (torrent *Torrent) Size() string {
	return humanize.Bytes(torrent.SizeWhenDone)
}

func (response *AddTorrentResponse) GetTorrentInfo() *TorrentInfo {
	torrentInfo := response.Arguments.TorrentAdded
	if torrentInfo == nil {
		torrentInfo = response.Arguments.TorrentDuplicate
	}
	return torrentInfo
}

func (tr *TransmissionClient) AddTorrent(magnet string) (*AddTorrentResponse, error) {
	addTorrentResponse := &AddTorrentResponse{}

	request := &Request{}
	request.Method = "torrent-add"
	request.Arguments.Filename = magnet

	data, err := tr.makeRpc(request)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, &addTorrentResponse); err != nil {
		return nil, err
	}

	return addTorrentResponse, nil
}

func (tr *TransmissionClient) GetTorrents(ids ...string) (*GetTorrentsResponse, error) {
	getTorrentsResponse := &GetTorrentsResponse{}

	request := &Request{}
	request.Method = "torrent-get"
	request.Arguments.Ids = append(request.Arguments.Ids, ids...)
	request.Arguments.Fields = []string{
		"id", "hashString", "name",
		"error", "errorString",
		"haveValid", "haveUnchecked", "eta",
		"leftUntilDone", "percentDone", "rateDownload", "rateUpload", "sizeWhenDone", "status",
	}

	data, err := tr.makeRpc(request)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, &getTorrentsResponse); err != nil {
		return nil, err
	}

	return getTorrentsResponse, nil
}

// Client
func (tr *TransmissionClient) makeRpc(request *Request) ([]byte, error) {
	req, err := json.Marshal(request)
	if err != nil {
		return make([]byte, 0), err
	}

	log.Printf("Request = %s", req)

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	makeRequest := func(token string) (*http.Response, error) {
		req, err := http.NewRequest("POST", tr.url, bytes.NewBuffer(req))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")

		if token != "" {
			req.Header.Set("X-Transmission-Session-Id", token)
		}

		return client.Do(req)
	}

	resp, err := makeRequest(tr.token)
	if err != nil {
		return make([]byte, 0), err
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		log.Printf("Transmission returned %d. Retrying with new session id.", resp.StatusCode)

		tr.token = resp.Header.Get("X-Transmission-Session-Id")

		//resp.Body.Close()

		resp, err = makeRequest(tr.token)
		if err != nil {
			return make([]byte, 0), err
		}
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return make([]byte, 0), err
	}

	log.Printf("Response = %s", data)

	return data, nil
}

type TransmissionClient struct {
	url   string
	token string
}

func CreateTransmissionClient(url string) *TransmissionClient {
	return &TransmissionClient{
		url: url,
	}
}
