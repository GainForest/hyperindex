import { describe, expect, it } from "vitest";
import { buildConnectionQueryDocument } from "./query-builder";

describe("buildConnectionQueryDocument", () => {
  it("builds a connection query with variables and selected fields", () => {
    expect(
      buildConnectionQueryDocument({
        fieldName: "orgHypercertsClaimActivity",
        arguments: [
          { name: "first", type: "Int" },
          { name: "where", type: "OrgHypercertsClaimActivityWhereInput" },
        ],
        selectedFields: ["uri", "did", "claimId"],
      })
    ).toMatchInlineSnapshot(`
      "query QueryBuilder($first: Int, $where: OrgHypercertsClaimActivityWhereInput) {
        orgHypercertsClaimActivity(first: $first, where: $where) {
          totalCount
          pageInfo {
            hasNextPage
            hasPreviousPage
            startCursor
            endCursor
          }
          edges {
            cursor
            node {
              uri
              did
              claimId
            }
          }
        }
      }"
    `);
  });

  it("falls back to common metadata fields", () => {
    const document = buildConnectionQueryDocument({
      fieldName: "records",
      arguments: [{ name: "collection", type: "String!" }],
      selectedFields: [],
    });

    expect(document).toContain("collection: $collection");
    expect(document).toContain("uri\n        cid\n        did\n        rkey");
  });
});
