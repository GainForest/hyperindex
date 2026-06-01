export type QueryBuilderArgument = {
  name: string;
  variableName?: string;
  type: string;
};

export type QueryBuilderDocumentOptions = {
  operationName?: string;
  fieldName: string;
  arguments: QueryBuilderArgument[];
  selectedFields: string[];
  includePageInfo?: boolean;
  includeTotalCount?: boolean;
};

function indentBlock(value: string, spaces: number): string {
  const padding = " ".repeat(spaces);
  return value
    .split("\n")
    .map((line) => `${padding}${line}`)
    .join("\n");
}

export function buildConnectionQueryDocument({
  operationName = "QueryBuilder",
  fieldName,
  arguments: args,
  selectedFields,
  includePageInfo = true,
  includeTotalCount = true,
}: QueryBuilderDocumentOptions): string {
  const variableDefinitions = args
    .map((arg) => `$${arg.variableName ?? arg.name}: ${arg.type}`)
    .join(", ");
  const fieldArguments = args
    .map((arg) => `${arg.name}: $${arg.variableName ?? arg.name}`)
    .join(", ");
  const operationSignature = variableDefinitions
    ? `query ${operationName}(${variableDefinitions})`
    : `query ${operationName}`;
  const fieldSignature = fieldArguments ? `${fieldName}(${fieldArguments})` : fieldName;
  const nodeFields = selectedFields.length > 0 ? selectedFields : ["uri", "cid", "did", "rkey"];

  const connectionSelections: string[] = [];
  if (includeTotalCount) {
    connectionSelections.push("totalCount");
  }
  if (includePageInfo) {
    connectionSelections.push(`pageInfo {\n${indentBlock("hasNextPage\nhasPreviousPage\nstartCursor\nendCursor", 2)}\n}`);
  }
  connectionSelections.push(`edges {\n  cursor\n  node {\n${indentBlock(nodeFields.join("\n"), 4)}\n  }\n}`);

  return `${operationSignature} {\n  ${fieldSignature} {\n${indentBlock(connectionSelections.join("\n"), 4)}\n  }\n}`;
}

export function stableStringify(value: unknown): string {
  return JSON.stringify(value, null, 2);
}
