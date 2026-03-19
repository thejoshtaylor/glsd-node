module.exports = {
  apps: [
    {
      name: "telegram-claude",
      script: "./node_modules/tsx/dist/cli.mjs",
      args: "src/index.ts",
      interpreter: "node",
      cwd: __dirname,
      restart_delay: 5000,
      max_restarts: 50,
      min_uptime: 10000,
      exp_backoff_restart_delay: 1000,
      watch: false,
      windowsHide: true,
      out_file: "./logs/out.log",
      error_file: "./logs/error.log",
      merge_logs: true,
      env: {
        NODE_ENV: "production",
      },
    },
  ],
};
