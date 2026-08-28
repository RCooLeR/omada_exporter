export interface CustomCardMetadata {
  type: string;
  name: string;
  description: string;
}

interface CustomCardHost {
  customCards?: CustomCardMetadata[];
}

declare global {
  interface Window {
    customCards?: CustomCardMetadata[];
  }
}

/** Register a Lovelace card without throwing or duplicating its picker entry on resource reload. */
export function registerCustomCard(
  registry: Pick<CustomElementRegistry, "define" | "get">,
  host: CustomCardHost,
  tagName: string,
  constructor: CustomElementConstructor,
  metadata: CustomCardMetadata
): void {
  if (!registry.get(tagName)) {
    registry.define(tagName, constructor);
  }

  const cards = host.customCards ?? [];
  if (!cards.some((card) => card.type === metadata.type)) {
    cards.push(metadata);
  }
  host.customCards = cards;
}
