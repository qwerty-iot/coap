package coap

func handleConfirmable(req *Message) *Message {
	var rsp *Message

	if req.Code == 0 {
		rsp = &Message{
			Type:      TypeReset,
			Code:      0,
			MessageID: req.MessageID,
			Token:     req.Token,
		}
		return rsp
	}

	callback := matchRoutes(req)
	if callback!=nil {
		rsp = callback(req)
	} else {
		rsp = &Message{
			Type:      TypeAcknowledgement,
			Code:      RspCodeNotFound,
			MessageID: req.MessageID,
			Token:     req.Token,
		}
	}

	return rsp
}
