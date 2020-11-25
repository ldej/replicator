package main

type PeerError struct {
	PeerID       string `json:"peerId"`
	ClusterID    string `json:"clusterID"`
	ErrorMessage string `json:"error"`
}

type PeerErrors []PeerError

func MapPeerErrors(peers IDs, errors []error) PeerErrors {
	var peerErrors PeerErrors
	for i, err := range errors {
		if err != nil {
			peerErrors = append(peerErrors, PeerError{
				PeerID:       peers[i].ID.String(),
				ClusterID:    peers[i].ClusterID,
				ErrorMessage: err.Error(),
			})
		}
	}
	return peerErrors
}
