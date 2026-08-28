import { describe, expect, it, vi } from "vitest";
import { registerCustomCard, type CustomCardMetadata } from "./register-card";

describe("registerCustomCard", () => {
  it("defines and advertises a card only once across resource reloads", () => {
    const constructor = class {} as unknown as CustomElementConstructor;
    let registered: CustomElementConstructor | undefined;
    const define = vi.fn((_name: string, value: CustomElementConstructor) => {
      registered = value;
    });
    const registry = {
      define,
      get: vi.fn(() => registered)
    };
    const host: { customCards?: CustomCardMetadata[] } = {};
    const metadata = {
      type: "test-card",
      name: "Test Card",
      description: "A test card."
    };

    registerCustomCard(registry, host, "test-card", constructor, metadata);
    registerCustomCard(registry, host, "test-card", constructor, metadata);

    expect(define).toHaveBeenCalledOnce();
    expect(host.customCards).toEqual([metadata]);
  });
});
