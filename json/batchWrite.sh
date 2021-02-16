# aws dynamodb batch-write-item \
#   --request-items file://data7a.json \
#     --return-consumed-capacity TOTAL
# aws dynamodb batch-write-item \
#   --request-items file://data7b.json \
#     --return-consumed-capacity TOTAL
# aws dynamodb batch-write-item \
#     --request-items file://data7c.json \
#     --return-consumed-capacity TOTAL
aws dynamodb batch-write-item \
  --request-items file://Types.Relationship.json \
  --return-consumed-capacity TOTAL
aws dynamodb batch-write-item \
  --request-items file://Types.Movie.json \
  --return-consumed-capacity TOTAL
aws dynamodb batch-write-item \
  --request-items file://Types.Scalar.json \
  --return-consumed-capacity TOTAL
# aws dynamodb batch-write-item \
#   --request-items file://Types.json \
#   --return-consumed-capacity TOTAL