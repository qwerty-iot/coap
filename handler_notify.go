package coap

func handleNotify(req *Message) *Message {
	var rsp *Message

	c := getObserve(req)

	if c == nil {
		logDebug(nil, "coap: observation not found")
		rsp = &Message{
			Type:      TypeReset,
			Code:      req.Code,
			MessageID: req.MessageID,
			Token:     req.Token,
		}
		return rsp
	}

	err := c.callback(req, c.arg)
	if err != nil {
		logWarn(nil, err, "coap: error processing observation")
		rsp = &Message{
			Type:      TypeReset,
			Code:      req.Code,
			MessageID: req.MessageID,
			Token:     req.Token,
		}
	} else {
		if req.Type == TypeConfirmable {
			rsp = &Message{
				Type:      TypeAcknowledgement,
				Code:      RspCodeChanged,
				MessageID: req.MessageID,
				Token:     req.Token,
			}
		}
	}
	return rsp
}
