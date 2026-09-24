//! A minimal HTTP/1.1 GET client for radio streams. General-purpose clients
//! reject Shoutcast v1 servers, which answer `ICY 200 OK`.

use std::io::{self, BufRead, BufReader, Read, Write};
use std::net::{TcpStream, ToSocketAddrs};
use std::sync::{Arc, OnceLock};
use std::time::Duration;

use rustls::pki_types::ServerName;
use rustls::{ClientConfig, ClientConnection, RootCertStore, StreamOwned};
use url::Url;

pub const TIMEOUT: Duration = Duration::from_secs(10);
const MAX_REDIRECTS: usize = 5;
const MAX_LINE: usize = 8 * 1024;

pub struct Response {
    pub url: Url,
    pub headers: Vec<(String, String)>,
    pub body: Box<dyn BufRead + Send>,
}

impl Response {
    pub fn header(&self, name: &str) -> Option<&str> {
        self.headers.iter().find(|(k, _)| k.eq_ignore_ascii_case(name)).map(|(_, v)| v.as_str())
    }
}

pub fn get(url: &str, extra_headers: &[(&str, &str)]) -> io::Result<Response> {
    let mut url = Url::parse(url).map_err(invalid)?;
    for _ in 0..=MAX_REDIRECTS {
        let (status, resp) = request(&url, extra_headers)?;
        if (300..400).contains(&status)
            && let Some(location) = resp.header("location")
        {
            url = url.join(location).map_err(invalid)?;
            continue;
        }
        if !(200..300).contains(&status) {
            return Err(io::Error::other(format!("HTTP {status}")));
        }
        return Ok(resp);
    }
    Err(io::Error::other("too many redirects"))
}

trait Conn: Read + Write + Send {}
impl<T: Read + Write + Send> Conn for T {}

fn request(url: &Url, extra_headers: &[(&str, &str)]) -> io::Result<(u16, Response)> {
    let host = url.host_str().ok_or_else(|| invalid("no host in URL"))?;
    let https = match url.scheme() {
        "https" => true,
        "http" => false,
        scheme => return Err(invalid(format!("unsupported scheme {scheme}"))),
    };
    let port = url.port_or_known_default().unwrap_or(80);
    let tcp = connect(host, port)?;

    let mut conn: Box<dyn Conn> = if https {
        let name = ServerName::try_from(host.trim_matches(['[', ']']).to_string()).map_err(invalid)?;
        let tls = ClientConnection::new(tls_config(), name).map_err(io::Error::other)?;
        Box::new(StreamOwned::new(tls, tcp))
    } else {
        Box::new(tcp)
    };

    let host_header = match url.port() {
        Some(port) => format!("{host}:{port}"),
        None => host.to_string(),
    };
    let path = &url[url::Position::BeforePath..url::Position::AfterQuery];
    let mut req = format!(
        "GET {path} HTTP/1.1\r\nHost: {host_header}\r\nUser-Agent: goradion/{}\r\nAccept: */*\r\nConnection: close\r\n",
        env!("CARGO_PKG_VERSION")
    );
    for (k, v) in extra_headers {
        req.push_str(&format!("{k}: {v}\r\n"));
    }
    req.push_str("\r\n");
    conn.write_all(req.as_bytes())?;
    conn.flush()?;

    let mut reader = BufReader::with_capacity(64 * 1024, conn);
    let status = parse_status(&read_line(&mut reader)?)?;
    let mut headers = Vec::new();
    loop {
        let line = read_line(&mut reader)?;
        if line.is_empty() {
            break;
        }
        if let Some((k, v)) = line.split_once(':') {
            headers.push((k.trim().to_string(), v.trim().to_string()));
        }
    }

    let mut resp = Response { url: url.clone(), headers, body: Box::new(io::empty()) };
    let chunked = resp.header("transfer-encoding").is_some_and(|v| v.to_ascii_lowercase().contains("chunked"));
    let length = resp.header("content-length").and_then(|v| v.parse::<u64>().ok());
    resp.body = if chunked {
        Box::new(BufReader::new(Chunked { inner: reader, left: 0, done: false }))
    } else if let Some(n) = length {
        Box::new(reader.take(n))
    } else {
        Box::new(reader)
    };
    Ok((status, resp))
}

fn connect(host: &str, port: u16) -> io::Result<TcpStream> {
    let mut last = io::Error::other(format!("can't resolve {host}"));
    for addr in (host.trim_matches(['[', ']']), port).to_socket_addrs()? {
        match TcpStream::connect_timeout(&addr, TIMEOUT) {
            Ok(tcp) => {
                tcp.set_read_timeout(Some(TIMEOUT))?;
                tcp.set_write_timeout(Some(TIMEOUT))?;
                return Ok(tcp);
            }
            Err(e) => last = e,
        }
    }
    Err(last)
}

fn tls_config() -> Arc<ClientConfig> {
    static CONFIG: OnceLock<Arc<ClientConfig>> = OnceLock::new();
    CONFIG
        .get_or_init(|| {
            let roots = RootCertStore { roots: webpki_roots::TLS_SERVER_ROOTS.to_vec() };
            let provider = Arc::new(rustls::crypto::ring::default_provider());
            let config = ClientConfig::builder_with_provider(provider)
                .with_safe_default_protocol_versions()
                .expect("ring supports the default TLS versions")
                .with_root_certificates(roots)
                .with_no_client_auth();
            Arc::new(config)
        })
        .clone()
}

/// Accepts `HTTP/1.x 200 OK` and Shoutcast's `ICY 200 OK`.
fn parse_status(line: &str) -> io::Result<u16> {
    let mut parts = line.split_whitespace();
    let proto = parts.next().unwrap_or("");
    if !proto.starts_with("HTTP/") && proto != "ICY" {
        return Err(invalid(format!("not an HTTP response: {line:?}")));
    }
    parts.next().and_then(|s| s.parse().ok()).ok_or_else(|| invalid(format!("bad status line: {line:?}")))
}

fn read_line(r: &mut impl BufRead) -> io::Result<String> {
    let mut buf = Vec::new();
    r.take(MAX_LINE as u64).read_until(b'\n', &mut buf)?;
    if buf.last() != Some(&b'\n') {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "connection closed in headers"));
    }
    Ok(String::from_utf8_lossy(&buf).trim_end_matches(['\r', '\n']).to_string())
}

struct Chunked<R> {
    inner: R,
    left: usize,
    done: bool,
}

impl<R: BufRead> Read for Chunked<R> {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        if self.done || buf.is_empty() {
            return Ok(0);
        }
        if self.left == 0 {
            let line = read_line(&mut self.inner)?;
            let size = line.split(';').next().unwrap_or("").trim();
            self.left = usize::from_str_radix(size, 16).map_err(invalid)?;
            if self.left == 0 {
                self.done = true;
                return Ok(0);
            }
        }
        let n = buf.len().min(self.left);
        let n = self.inner.read(&mut buf[..n])?;
        if n == 0 {
            return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "truncated chunk"));
        }
        self.left -= n;
        if self.left == 0 {
            read_line(&mut self.inner)?;
        }
        Ok(n)
    }
}

fn invalid(e: impl ToString) -> io::Error {
    io::Error::new(io::ErrorKind::InvalidData, e.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn status_lines() {
        assert_eq!(parse_status("HTTP/1.1 302 Found").unwrap(), 302);
        assert_eq!(parse_status("ICY 200 OK").unwrap(), 200);
        assert!(parse_status("<html>").is_err());
    }

    #[test]
    fn chunked_body() {
        let raw = b"4\r\nWiki\r\n5;x=y\r\npedia\r\n0\r\n\r\n";
        let mut body = String::new();
        Chunked { inner: &raw[..], left: 0, done: false }.read_to_string(&mut body).unwrap();
        assert_eq!(body, "Wikipedia");
    }
}
