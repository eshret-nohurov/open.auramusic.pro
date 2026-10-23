// pm2 process for open.auramusic.pro. Runs the current release; nginx proxies the domain to it.
// The binary reads /opt/bm-open/.env (cwd) on start.
module.exports = {
  apps: [
    {
      name: 'bm-open',
      cwd: '/opt/bm-open',
      script: 'live/bm-open',
      args: 'serve',
      interpreter: 'none',
      max_memory_restart: '300M',
      // The Go runtime collects harder near this limit instead of growing up
      // to the restart threshold above (image cache 128 MB + decoding).
      env: { GOMEMLIMIT: '240MiB' },
      // The server drains requests and writes the running stats hour on SIGINT.
      kill_timeout: 10000,
    },
  ],
};
