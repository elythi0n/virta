// Package obsws implements an obs-websocket v5 client and a pipeline sink that pushes
// live chat stats and fires scene/source actions in response to chat events.
package obsws

import "encoding/json"

// Op codes defined by the obs-websocket v5 specification.
const (
	opHello           = 0
	opIdentify        = 1
	opIdentified      = 2
	opEvent           = 5
	opRequest         = 6
	opRequestResponse = 7
)

// message is the top-level obs-websocket frame envelope.
type message struct {
	Op   int             `json:"op"`
	Data json.RawMessage `json:"d"`
}

// helloData is the payload of the Hello frame (op=0).
type helloData struct {
	OBSWebSocketVersion string         `json:"obsWebSocketVersion"`
	RPCVersion          int            `json:"rpcVersion"`
	Authentication      *authChallenge `json:"authentication,omitempty"`
}

// authChallenge carries the PBKDF2-style challenge parameters sent by the server.
type authChallenge struct {
	Challenge string `json:"challenge"`
	Salt      string `json:"salt"`
}

// identifyData is the payload of the Identify frame (op=1).
type identifyData struct {
	RPCVersion         int    `json:"rpcVersion"`
	Authentication     string `json:"authentication,omitempty"`
	EventSubscriptions int    `json:"eventSubscriptions,omitempty"`
}

// requestData is the payload of a Request frame (op=6).
type requestData struct {
	Type    string          `json:"requestType"`
	ID      string          `json:"requestId"`
	Payload json.RawMessage `json:"requestData,omitempty"`
}

// responseData is the payload of a RequestResponse frame (op=7).
type responseData struct {
	Type    string          `json:"requestType"`
	ID      string          `json:"requestId"`
	Status  requestStatus   `json:"requestStatus"`
	Payload json.RawMessage `json:"responseData,omitempty"`
}

// requestStatus reports whether a request succeeded.
type requestStatus struct {
	Result  bool   `json:"result"`
	Code    int    `json:"code"`
	Comment string `json:"comment,omitempty"`
}

// getVersionResponse is the responseData payload for GetVersion.
type getVersionResponse struct {
	OBSVersion          string `json:"obsVersion"`
	OBSWebSocketVersion string `json:"obsWebSocketVersion"`
}

// getSceneListResponse is the responseData payload for GetSceneList.
type getSceneListResponse struct {
	CurrentProgramSceneName string      `json:"currentProgramSceneName"`
	Scenes                  []sceneItem `json:"scenes"`
}

// sceneItem is one entry in the scene list.
type sceneItem struct {
	SceneIndex int    `json:"sceneIndex"`
	SceneName  string `json:"sceneName"`
}

// getInputListResponse is the responseData payload for GetInputList.
type getInputListResponse struct {
	Inputs []inputItem `json:"inputs"`
}

// inputItem is one entry in the input list.
type inputItem struct {
	InputName string `json:"inputName"`
	InputKind string `json:"inputKind"`
}
