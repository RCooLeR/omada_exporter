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

```bash
npm install
npm run typecheck
npm test
npm run build
```

The production bundle is written to `dist/omada-network-card.js`.

## License

MIT License. See [../LICENSE](../LICENSE). See [../NOTICE](../NOTICE) for trademark and affiliation notice.
