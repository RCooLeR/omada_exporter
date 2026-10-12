# Omada Home Assistant Cards

`ha-cards` builds optional Lovelace cards for OmadaBridge Home Assistant MQTT entities.

It ships:

- `custom:omada-network-card`
- `custom:omada-links-card`

## Documentation

- [Card documentation index](./docs/index.md)
- [Installation](./docs/installation.md)
- [Card configuration](./docs/cards.md)
- [Data source and entity expectations](./docs/data-source.md)
- [Bridge Home Assistant guide](../bridge/docs/home-assistant.md)

## Development

Use Node.js 24.15 or newer in the 24 LTS series (recommended), Node.js 22.22.2 or newer in the 22 series, or Node.js 26+. Install the pinned npm 12.2.0 version before installing dependencies.

```bash
npm install --global npm@12.2.0
npm ci
npm run typecheck
npm test
npm run build
```

The production bundle is written to `dist/omada-network-card.js`.

## License

MIT License. See [../LICENSE](../LICENSE). See [../NOTICE](../NOTICE) for trademark and affiliation notice.
