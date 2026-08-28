import { describe, expect, it } from "vitest";
import type { MediaAsset, MetadataView } from "../services/metadata";
import {
  busyState,
  clean,
  facts,
  galleryShots,
  hubAction,
  isGameMissing,
  joinLimited,
  languageLabel,
  metaLine,
  metaStatus,
  orderPlatforms,
  pickHero,
  preferView,
  primaryAction,
  stepIndex,
  summaryView,
  tagList,
  type HubStatus,
  type MissingStatus,
  cancelFreesDisk,
} from "./view";

function view(patch: Partial<MetadataView> = {}): MetadataView {
  return {
    game: { id: "g1", title: "", sortTitle: "", createdAt: "" },
    cover: "",
    hero: "",
    screenshots: [],
    resolved: false,
    stale: false,
    provider: "",
    match: "idle",
    ...patch,
  };
}

function shot(patch: Partial<MediaAsset> = {}): MediaAsset {
  return {
    id: "s1",
    gameId: "g1",
    type: "screenshot",
    sourceUrl: "",
    path: "",
    url: "http://local/s1.png",
    width: 1920,
    height: 1080,
    createdAt: "",
    ...patch,
  };
}

function status(patch: Partial<HubStatus> = {}): HubStatus {
  return {
    installed: false,
    running: false,
    updateAvailable: false,
    releaseCount: 0,
    releasesLoading: false,
    busy: null,
    terminalDownload: null,
    ...patch,
  };
}

describe("clean", () => {
  it.each([
    [undefined, ""],
    [null, ""],
    ["  ", ""],
    ["N/A", ""],
    ["Unknown", ""],
    ["—", ""],
    ["undefined", ""],
    ["  CD Projekt RED ", "CD Projekt RED"],
    [2020, "2020"],
  ])("%p becomes %p", (input, expected) => {
    expect(clean(input)).toBe(expected);
  });
});

describe("pickHero", () => {
  it("prefers the stored hero", () => {
    expect(pickHero("http://local/hero.png", [shot()])).toBe(
      "http://local/hero.png",
    );
  });

  it("falls back to a landscape screenshot", () => {
    expect(
      pickHero("", [
        shot({ id: "p", width: 600, height: 900 }),
        shot({ id: "l" }),
      ]),
    ).toBe("http://local/s1.png");
  });

  it("refuses portrait-only artwork", () => {
    expect(pickHero("", [shot({ width: 600, height: 900 })])).toBe("");
  });

  it("refuses screenshots without a url", () => {
    expect(pickHero("", [shot({ url: "" })])).toBe("");
  });

  it("has no hero without artwork at all", () => {
    expect(pickHero("", [])).toBe("");
  });
});

describe("galleryShots", () => {
  it("drops the shot already used as hero", () => {
    const shots = [
      shot({ id: "a", url: "http://local/a.png" }),
      shot({ id: "b", url: "http://local/b.png" }),
    ];
    expect(galleryShots(shots, "http://local/a.png").map((s) => s.id)).toEqual([
      "b",
    ]);
  });

  it("drops shots without a url", () => {
    expect(galleryShots([shot({ url: "" })], "")).toEqual([]);
  });
});

describe("metaLine", () => {
  it("collects only useful parts", () => {
    expect(
      metaLine({
        year: 2020,
        developer: "CD Projekt RED",
        publisher: "CD Projekt",
        genres: ["RPG", "Shooter"],
        platforms: ["Windows"],
      }),
    ).toEqual(["2020", "CD Projekt RED", "RPG", "Windows"]);
  });

  it("leaves no gaps when fields are missing", () => {
    expect(
      metaLine({
        year: null,
        developer: "",
        publisher: "Valve",
        genres: [],
        platforms: undefined,
      }),
    ).toEqual(["Valve"]);
  });

  it("is empty without metadata", () => {
    expect(metaLine({})).toEqual([]);
  });

  it("does not repeat the same value", () => {
    expect(metaLine({ developer: "Valve", genres: ["valve"] })).toEqual([
      "Valve",
    ]);
  });
});

describe("facts", () => {
  it("drops empty and meaningless values", () => {
    expect(
      facts([
        { label: "Версия", value: "" },
        { label: "Размер", value: "N/A" },
        { label: "Разработчик", value: "Valve" },
      ]),
    ).toEqual([{ label: "Разработчик", value: "Valve" }]);
  });

  it("keeps the full value when it differs from the shown one", () => {
    expect(
      facts([
        {
          label: "Расположение",
          value: "C:…\\Game",
          full: "C:\\Games\\Game",
          mono: true,
        },
      ]),
    ).toEqual([
      {
        label: "Расположение",
        value: "C:…\\Game",
        mono: true,
        full: "C:\\Games\\Game",
      },
    ]);
  });

  it("omits the full value when it matches", () => {
    expect(
      facts([{ label: "Путь", value: "C:\\Game", full: "C:\\Game" }]),
    ).toEqual([{ label: "Путь", value: "C:\\Game" }]);
  });
});

describe("tagList", () => {
  it("merges groups without duplicates", () => {
    expect(
      tagList([
        ["RPG", "Action"],
        ["action", "Fantasy"],
      ]),
    ).toEqual(["RPG", "Action", "Fantasy"]);
  });

  it("respects the limit", () => {
    expect(tagList([["a", "b", "c"]], 2)).toEqual(["a", "b"]);
  });

  it("is empty for empty input", () => {
    expect(tagList([undefined, null, []])).toEqual([]);
  });
});

describe("orderPlatforms", () => {
  it("puts the desktop platform first", () => {
    expect(
      orderPlatforms([
        "PlayStation 4",
        "Linux",
        "PC (Microsoft Windows)",
        "Mac",
      ]),
    ).toEqual(["PC (Microsoft Windows)", "PlayStation 4", "Linux", "Mac"]);
  });

  it("keeps the order when there is no desktop platform", () => {
    expect(orderPlatforms(["PlayStation 4", "Mac"])).toEqual([
      "PlayStation 4",
      "Mac",
    ]);
  });

  it("does not match a platform that merely contains pc", () => {
    expect(orderPlatforms(["Arcade", "PC Engine"])[0]).toBe("PC Engine");
    expect(orderPlatforms(["Epcot", "Mac"])).toEqual(["Epcot", "Mac"]);
  });

  it("is empty without platforms", () => {
    expect(orderPlatforms(undefined)).toEqual([]);
  });
});

describe("joinLimited", () => {
  it("joins a short list whole", () => {
    expect(joinLimited(["a", "b"], 4)).toBe("a, b");
  });

  it("counts the rest", () => {
    expect(joinLimited(["a", "b", "c", "d", "e", "f"], 4)).toBe(
      "a, b, c, d +2",
    );
  });

  it("is empty for an empty list", () => {
    expect(joinLimited([], 4)).toBe("");
  });
});

describe("summaryView", () => {
  it("keeps a short summary whole", () => {
    expect(summaryView("Короткое описание.", false)).toEqual({
      text: "Короткое описание.",
      expandable: false,
    });
  });

  it("reports nothing to show when the summary is missing", () => {
    expect(summaryView("", false)).toEqual({ text: "", expandable: false });
  });

  it("cuts a long summary on a word boundary", () => {
    const long = "слово ".repeat(200) + "конец";
    const view = summaryView(long, false);
    expect(view.expandable).toBe(true);
    expect(view.text.endsWith("…")).toBe(true);
    expect(view.text.length).toBeLessThan(long.length);
    expect(view.text).not.toContain(" …");
  });

  it("returns the whole summary once expanded", () => {
    const long = "слово ".repeat(200) + "конец";
    expect(summaryView(long, true)).toEqual({
      text: long.trim(),
      expandable: true,
    });
  });
});

describe("primaryAction", () => {
  it("offers Play for an installed game", () => {
    expect(primaryAction(status({ installed: true }))).toEqual({
      kind: "play",
      label: "Играть",
      disabled: false,
    });
  });

  it("offers Stop for a running game", () => {
    expect(primaryAction(status({ installed: true, running: true })).kind).toBe(
      "stop",
    );
  });

  it("offers Update when an update is available", () => {
    expect(
      primaryAction(status({ installed: true, updateAvailable: true })).kind,
    ).toBe("update");
  });

  it("offers Install for a game with releases", () => {
    expect(primaryAction(status({ releaseCount: 3 })).kind).toBe("install");
  });

  it("waits instead of guessing while releases load", () => {
    expect(primaryAction(status({ releasesLoading: true })).kind).toBe(
      "resolving",
    );
  });

  it("reports no downloads when nothing is offered", () => {
    const action = primaryAction(status());
    expect(action.kind).toBe("unavailable");
    expect(action.disabled).toBe(true);
  });

  it("shows progress instead of an action while busy", () => {
    const action = primaryAction(
      status({ installed: true, busy: { label: "Загрузка", progress: 0.4 } }),
    );
    expect(action).toEqual({
      kind: "progress",
      label: "Загрузка",
      disabled: true,
      progress: 0.4,
    });
  });
});

describe("hubAction", () => {
  it("plays an installed game even with a stale terminal download nearby", () => {
    expect(
      hubAction(
        status({ installed: true, terminalDownload: { status: "failed" } }),
      ).kind,
    ).toBe("play");
  });

  it("shows progress while a download or install is active", () => {
    expect(
      hubAction(
        status({
          busy: { label: "Загрузка", progress: 0.3 },
          terminalDownload: { status: "completed" },
        }),
      ).kind,
    ).toBe("progress");
  });

  it("offers a retry for a failed download without releases", () => {
    expect(
      hubAction(status({ terminalDownload: { status: "failed" } })),
    ).toEqual({
      kind: "retry-download",
      label: "Повторить загрузку",
      disabled: false,
    });
  });

  it("offers to install a completed download without releases", () => {
    expect(
      hubAction(status({ terminalDownload: { status: "completed" } })),
    ).toEqual({
      kind: "install-download",
      label: "Установить",
      disabled: false,
    });
  });

  it("prefers installing the completed download over fresh releases", () => {
    expect(
      hubAction(
        status({ releaseCount: 3, terminalDownload: { status: "completed" } }),
      ).kind,
    ).toBe("install-download");
  });

  it("falls back to the release logic without a terminal download", () => {
    expect(hubAction(status()).kind).toBe("unavailable");
  });
});

function missingStatus(patch: Partial<MissingStatus> = {}): MissingStatus {
  return {
    hasLocalGame: false,
    hasCatalogGame: false,
    catalogLoading: false,
    hasOwnDownload: false,
    ...patch,
  };
}

describe("isGameMissing", () => {
  it("is missing when there is nothing at all", () => {
    expect(isGameMissing(missingStatus())).toBe(true);
  });

  it("is not missing while the catalog is still loading", () => {
    expect(isGameMissing(missingStatus({ catalogLoading: true }))).toBe(false);
  });

  it("is not missing with only a library record", () => {
    expect(isGameMissing(missingStatus({ hasLocalGame: true }))).toBe(false);
  });

  it("is not missing with only a catalog game", () => {
    expect(isGameMissing(missingStatus({ hasCatalogGame: true }))).toBe(false);
  });

  it("is not missing without a library or catalog record when a download owns the page", () => {
    expect(isGameMissing(missingStatus({ hasOwnDownload: true }))).toBe(false);
  });
});

describe("busyState", () => {
  it("takes the first active entry", () => {
    expect(
      busyState([
        { active: false, label: "Загрузка", progress: 0.1 },
        { active: true, label: "Установка", progress: 0.5 },
      ]),
    ).toEqual({ label: "Установка", progress: 0.5 });
  });

  it("clamps the progress", () => {
    expect(
      busyState([{ active: true, label: "Установка", progress: 4 }])?.progress,
    ).toBe(1);
    expect(
      busyState([{ active: true, label: "Установка", progress: Number.NaN }])
        ?.progress,
    ).toBe(0);
  });

  it("is null when nothing runs", () => {
    expect(
      busyState([null, undefined, { active: false, label: "x", progress: 0 }]),
    ).toBeNull();
  });
});

describe("stepIndex", () => {
  it.each([
    [0, 3, 1, 1],
    [2, 3, 1, 0],
    [0, 3, -1, 2],
    [1, 1, 1, 0],
  ])("index %i of %i by %i", (index, length, delta, expected) => {
    expect(stepIndex(index, length, delta)).toBe(expected);
  });

  it("stays at zero for an empty gallery", () => {
    expect(stepIndex(0, 0, 1)).toBe(0);
  });
});

describe("preferView", () => {
  it("keeps cached metadata when a refresh returns nothing", () => {
    const cached = view({
      game: { id: "g1", title: "Игра", sortTitle: "", createdAt: "" },
      cover: "c.png",
    });
    expect(preferView(cached, view())).toBe(cached);
  });

  it("accepts a view that carries content", () => {
    const cached = view({ cover: "c.png" });
    const fresh = view({ cover: "new.png" });
    expect(preferView(cached, fresh)).toBe(fresh);
  });

  it("accepts an empty view for another game", () => {
    const cached = view({ cover: "c.png" });
    const other = view({
      game: { id: "g2", title: "", sortTitle: "", createdAt: "" },
    });
    expect(preferView(cached, other)).toBe(other);
  });

  it("accepts anything when nothing was cached", () => {
    const fresh = view();
    expect(preferView(null, fresh)).toBe(fresh);
    expect(preferView(view(), fresh)).toBe(fresh);
  });
});

describe("languageLabel", () => {
  it("shortens codes and counts the rest", () => {
    expect(languageLabel(["ru", "en", "de", "fr", "it"])).toBe("RU, EN, DE +2");
  });

  it("keeps full names as they are", () => {
    expect(languageLabel(["Русский"])).toBe("Русский");
  });

  it("is empty without languages", () => {
    expect(languageLabel(undefined)).toBe("");
    expect(languageLabel([" ", ""])).toBe("");
  });
});

describe("cancelFreesDisk", () => {
  const cases: { status: "completed" | "failed"; want: boolean }[] = [
    { status: "failed", want: true },
    { status: "completed", want: false },
  ];

  for (const c of cases) {
    it(`reports ${c.want} for a ${c.status} download`, () => {
      expect(cancelFreesDisk(c.status)).toBe(c.want);
    });
  }
});

describe("metaStatus", () => {
  const cases: { name: string; input: Parameters<typeof metaStatus>[0]; want: string }[] = [
    {
      name: "keeps the card quiet while the provider is missing",
      input: { available: false, busy: true, match: "unmatched" },
      want: "ready",
    },
    {
      name: "keeps the card quiet for a matched game",
      input: { available: true, busy: false, match: "failed", resolved: true },
      want: "ready",
    },
    {
      name: "reports a lookup started by the card itself",
      input: { available: true, busy: true, match: "idle" },
      want: "searching",
    },
    {
      name: "reports a lookup already running in the background",
      input: { available: true, busy: false, match: "searching" },
      want: "searching",
    },
    {
      name: "asks for a manual match once the search came back empty",
      input: { available: true, busy: false, match: "unmatched" },
      want: "unmatched",
    },
    {
      name: "offers a retry after a provider error",
      input: { available: true, busy: false, match: "failed" },
      want: "failed",
    },
    {
      name: "stays out of the way once the user gave up",
      input: { available: true, busy: false, match: "skipped" },
      want: "skipped",
    },
    {
      name: "shows nothing for an unresolved game nobody has looked up yet",
      input: { available: true, busy: false, match: "idle" },
      want: "ready",
    },
    {
      name: "shows nothing without a view",
      input: { available: true, busy: false, match: null },
      want: "ready",
    },
  ];

  for (const c of cases) {
    it(c.name, () => {
      expect(metaStatus(c.input)).toBe(c.want);
    });
  }
});
