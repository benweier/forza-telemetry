import { expect, test } from "vitest";
import { gearLabel } from "./format";

test("gearLabel maps reverse (0) and forward gears", () => {
  expect(gearLabel(0)).toBe("R"); // Forza encodes reverse as gear 0
  expect(gearLabel(1)).toBe("1");
  expect(gearLabel(8)).toBe("8");
});
