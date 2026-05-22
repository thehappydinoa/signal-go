package web

import "net/url"

// Default CDN hostnames keyed by AttachmentPointer.cdnNumber.
var DefaultCDNHosts = map[uint32]string{
	0: "https://cdn.signal.org",
	2: "https://cdn2.signal.org",
	3: "https://cdn3.signal.org",
}

// CDNAttachmentURL returns the download URL for a cdnKey on the given CDN.
func CDNAttachmentURL(cdnNumber uint32, cdnKey string) (string, error) {
	return cdnAttachmentURL(DefaultCDNHosts, cdnNumber, cdnKey)
}

func (c *Client) cdnAttachmentURL(cdnNumber uint32, cdnKey string) (string, error) {
	hosts := DefaultCDNHosts
	if len(c.CDNHosts) > 0 {
		hosts = c.CDNHosts
	}
	return cdnAttachmentURL(hosts, cdnNumber, cdnKey)
}

func cdnAttachmentURL(hosts map[uint32]string, cdnNumber uint32, cdnKey string) (string, error) {
	host, ok := hosts[cdnNumber]
	if !ok {
		host = hosts[3]
		if host == "" {
			host = DefaultCDNHosts[3]
		}
	}
	return host + "/attachments/" + url.PathEscape(cdnKey), nil
}
