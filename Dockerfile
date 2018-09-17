FROM scratch
ADD main /
ADD namespaceTemplate.yaml /
EXPOSE 3000
CMD ["/main"]
