package exchange

import (
	"encoding/json"
	"fmt"
	"github.com/open-horizon/anax/cli/cliutils"
	"github.com/open-horizon/anax/exchange"
	"net/http"
)

// We only care about handling the node names, so the rest is left as interface{} and will be passed from the exchange to the display
type ExchangeNodes struct {
	LastIndex int                    `json:"lastIndex"`
	Nodes     map[string]interface{} `json:"nodes"`
}

func NodeList(org string, userPw string, node string, namesOnly bool) {
	cliutils.SetWhetherUsingApiKey(userPw)
	org, node = cliutils.TrimOrg(org, node)
	if namesOnly && node == "" {
		// Only display the names
		var resp ExchangeNodes
		cliutils.ExchangeGet(cliutils.GetExchangeUrl(), "orgs/"+org+"/nodes"+cliutils.AddSlash(node), cliutils.OrgAndCreds(org, userPw), []int{200, 404}, &resp)
		nodes := []string{}
		for n := range resp.Nodes {
			nodes = append(nodes, n)
		}
		jsonBytes, err := json.MarshalIndent(nodes, "", cliutils.JSON_INDENT)
		if err != nil {
			cliutils.Fatal(cliutils.JSON_PARSING_ERROR, "failed to marshal 'exchange node list' output: %v", err)
		}
		fmt.Printf("%s\n", jsonBytes)
	} else {
		// Display the full resources
		var nodes ExchangeNodes
		httpCode := cliutils.ExchangeGet(cliutils.GetExchangeUrl(), "orgs/"+org+"/nodes"+cliutils.AddSlash(node), cliutils.OrgAndCreds(org, userPw), []int{200, 404}, &nodes)
		if httpCode == 404 && node != "" {
			cliutils.Fatal(cliutils.NOT_FOUND, "node '%s' not found in org %s", node, org)
		}
		output := cliutils.MarshalIndent(nodes.Nodes, "exchange nodes list")
		fmt.Println(output)
	}
}

func NodeCreate(org, nodeIdTok, node, token, userPw, email string) {
	// They should specify either nodeIdTok (for backward compat) or node and token, but not both
	var nodeId, nodeToken string
	if node != "" || token != "" {
		if node == "" || token == "" {
			cliutils.Fatal(cliutils.CLI_INPUT_ERROR, "if node or token are specified then they both must be specified")
		}
		// at this point we know both node and token were specified
		if nodeIdTok != "" {
			cliutils.Fatal(cliutils.CLI_INPUT_ERROR, "do not specify both the -n flag and the node and token positional arguments. They mean the same thing.")
		}
		nodeId = node
		nodeToken = token
	} else {
		// here we know neither node nor token were specified
		if nodeIdTok == "" {
			cliutils.Fatal(cliutils.CLI_INPUT_ERROR, "either the node and token positional arguments, or the -n flag must be specified.")
		}
		nodeId, nodeToken = cliutils.SplitIdToken(nodeIdTok)
	}

	cliutils.SetWhetherUsingApiKey(userPw)
	exchUrlBase := cliutils.GetExchangeUrl()

	// Assume the user exists and try to create the node, but handle the error cases
	putNodeReq := exchange.PutDeviceRequest{Token: nodeToken, Name: nodeId, SoftwareVersions: make(map[string]string), PublicKey: []byte("")} // we only need to set the token
	httpCode := cliutils.ExchangePutPost(http.MethodPut, exchUrlBase, "orgs/"+org+"/nodes/"+nodeId, cliutils.OrgAndCreds(org, userPw), []int{201, 401, 403}, putNodeReq)

	if httpCode == 401 {
		// Invalid creds means the user doesn't exist, or pw is wrong, try to create it if we are in the public org
		user, pw := cliutils.SplitIdToken(userPw)
		if org == "public" && email != "" {
			// In the public org we can create a user anonymously, so try that
			fmt.Printf("User %s/%s does not exist in the exchange with the specified password, creating it...\n", org, user)
			postUserReq := cliutils.UserExchangeReq{Password: pw, Admin: false, Email: email}
			httpCode = cliutils.ExchangePutPost(http.MethodPost, exchUrlBase, "orgs/"+org+"/users/"+user, "", []int{201}, postUserReq)

			// User created, now try to create the node again
			httpCode = cliutils.ExchangePutPost(http.MethodPut, exchUrlBase, "orgs/"+org+"/nodes/"+nodeId, cliutils.OrgAndCreds(org, userPw), []int{201}, putNodeReq)
		} else {
			cliutils.Fatal(cliutils.CLI_INPUT_ERROR, "user '%s/%s' does not exist with the specified password or -e was not specified to be able to create it in the 'public' org.", org, user)
		}
	} else if httpCode == 403 {
		// Access denied means the node exists and is owned by another user. Figure out who and tell the user
		var nodesOutput exchange.GetDevicesResponse
		httpCode = cliutils.ExchangeGet(cliutils.GetExchangeUrl(), "orgs/"+org+"/nodes/"+nodeId, cliutils.OrgAndCreds(org, userPw), []int{200}, &nodesOutput)
		var ok bool
		var ourNode exchange.Device
		if ourNode, ok = nodesOutput.Devices[cliutils.OrgAndCreds(org, nodeId)]; !ok {
			cliutils.Fatal(cliutils.INTERNAL_ERROR, "key '%s' not found in exchange nodes output", cliutils.OrgAndCreds(org, nodeId))
		}
		cliutils.Fatal(cliutils.CLI_INPUT_ERROR, "can not update existing node %s because it is owned by another user (%s)", nodeId, ourNode.Owner)
	}
}

type NodeExchangePatchToken struct {
	Token string `json:"token"`
}

func NodeSetToken(org, userPw, node, token string) {
	cliutils.SetWhetherUsingApiKey(userPw)
	patchNodeReq := NodeExchangePatchToken{Token: token}
	cliutils.ExchangePutPost(http.MethodPatch, cliutils.GetExchangeUrl(), "orgs/"+org+"/nodes/"+node, cliutils.OrgAndCreds(org, userPw), []int{201}, patchNodeReq)
}

func NodeConfirm(org, node, token string) {
	cliutils.SetWhetherUsingApiKey("")
	httpCode := cliutils.ExchangeGet(cliutils.GetExchangeUrl(), "orgs/"+org+"/nodes/"+node, cliutils.OrgAndCreds(org, node+":"+token), []int{200}, nil)
	if httpCode == 200 {
		fmt.Println("Node id and token are valid.")
	}
	// else cliutils.ExchangeGet() already gave the error msg
}

func NodeRemove(org, userPw, node string, force bool) {
	cliutils.SetWhetherUsingApiKey(userPw)
	org, node = cliutils.TrimOrg(org, node)
	if !force {
		cliutils.ConfirmRemove("Are you sure you want to remove node '" + org + "/" + node + "' from the Horizon Exchange (should not be done while an edge node is registered with this node id)?")
	}

	httpCode := cliutils.ExchangeDelete(cliutils.GetExchangeUrl(), "orgs/"+org+"/nodes/"+node, cliutils.OrgAndCreds(org, userPw), []int{204, 404})
	if httpCode == 404 {
		cliutils.Fatal(cliutils.NOT_FOUND, "node '%s' not found in org %s", node, org)
	}
}
